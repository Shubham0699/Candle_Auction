// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {VRFCoordinatorV2Interface} from "lib/chainlink-brownie-contracts/contracts/src/v0.8/vrf/interfaces/VRFCoordinatorV2Interface.sol";
import {VRFConsumerBaseV2} from "lib/chainlink-brownie-contracts/contracts/src/v0.8/vrf/VRFConsumerBaseV2.sol";

contract CandleAuction is Ownable, VRFConsumerBaseV2 {
    enum AuctionPhase {
        NotStarted,
        Commit,
        Reveal,
        Ended
    }

    struct Bid {
        bytes32 commitment;
        uint256 amount;
        bool revealed;
    }

    uint256 public commitDeadline;
    uint256 public revealDeadline;
    uint256 public randomEndBlock;

    mapping(address => Bid) public bids;
    address[] public bidders;
    mapping(address => uint256) public revealedBids;

    bool public randomEndBlockRequested;
    uint256 public requestId;

    uint256 public highestBid;
    address public highestBidder;

    VRFCoordinatorV2Interface public vrfCoordinator;
    uint64 public subscriptionId;
    bytes32 public keyHash;
    uint32 public callbackGasLimit;
    uint16 public requestConfirmations;
    uint32 public numWords = 1;

    event AuctionStarted(uint256 commitDeadline, uint256 revealDeadline);
    event BidCommitted(address indexed bidder);
    event BidRevealed(address indexed bidder, uint256 amount);
    event AuctionEnded(address winner, uint256 amount);
    event RandomEndBlockRequested();
    event RandomEndBlockFulfilled(uint256 blockNumber);

    constructor(
        address _vrfCoordinator,
        uint64 _subscriptionId,
        bytes32 _keyHash,
        uint32 _callbackGasLimit,
        uint16 _requestConfirmations
    ) Ownable(msg.sender) VRFConsumerBaseV2(_vrfCoordinator) {
        vrfCoordinator = VRFCoordinatorV2Interface(_vrfCoordinator);
        subscriptionId = _subscriptionId;
        keyHash = _keyHash;
        callbackGasLimit = _callbackGasLimit;
        requestConfirmations = _requestConfirmations;
    }

    modifier onlyBefore(uint256 timestamp) {
        require(block.timestamp < timestamp, "Too late");
        _;
    }

    modifier onlyAfter(uint256 timestamp) {
        require(block.timestamp >= timestamp, "Too early");
        _;
    }

    modifier atPhase(AuctionPhase expected) {
        require(getCurrentPhase() == expected, "Invalid auction phase");
        _;
    }

    function startAuction(
        uint256 _commitDuration,
        uint256 _revealDuration
    ) external onlyOwner atPhase(AuctionPhase.NotStarted) {
        commitDeadline = block.timestamp + _commitDuration;
        revealDeadline = commitDeadline + _revealDuration;
        emit AuctionStarted(commitDeadline, revealDeadline);
    }

    function commitBid(
        bytes32 hashedBid
    ) external payable atPhase(AuctionPhase.Commit) onlyBefore(commitDeadline) {
        require(msg.value > 0, "Must send ETH with bid");
        require(bids[msg.sender].commitment == bytes32(0), "Already committed");

        bids[msg.sender] = Bid({
            commitment: hashedBid,
            amount: msg.value,
            revealed: false
        });

        bidders.push(msg.sender);
        emit BidCommitted(msg.sender);
    }

    function revealBid(
        uint256 amount,
        bytes32 salt
    ) external atPhase(AuctionPhase.Reveal) {
        require(block.timestamp <= revealDeadline, "Too late");

        Bid storage userBid = bids[msg.sender];
        require(userBid.commitment != bytes32(0), "No bid committed");
        require(!userBid.revealed, "Already revealed");

        bytes32 computedHash = keccak256(abi.encode(amount, salt));
        require(userBid.commitment == computedHash, "Commitment mismatch");
        require(
            userBid.amount == amount,
            "Reveal amount doesn't match committed amount"
        );

        userBid.revealed = true;
        revealedBids[msg.sender] = amount;

        if (amount > highestBid && block.timestamp <= randomEndBlock) {
            highestBid = amount;
            highestBidder = msg.sender;
        }

        emit BidRevealed(msg.sender, amount);
    }

    function requestRandomEndBlock()
        external
        onlyOwner
        atPhase(AuctionPhase.Reveal)
    {
        require(!randomEndBlockRequested, "Already requested");

        requestId = vrfCoordinator.requestRandomWords(
            keyHash,
            subscriptionId,
            requestConfirmations,
            callbackGasLimit,
            numWords
        );

        randomEndBlockRequested = true;
        emit RandomEndBlockRequested();
    }

    function fulfillRandomWords(
        uint256,
        uint256[] memory randomWords
    ) internal override {
        require(randomEndBlockRequested, "Random request not made");

        uint256 range = revealDeadline - commitDeadline;
        uint256 offset = randomWords[0] % range;
        randomEndBlock = commitDeadline + offset;

        emit RandomEndBlockFulfilled(randomEndBlock);
    }

    function settleAuction() external onlyAfter(revealDeadline) {
        require(getCurrentPhase() == AuctionPhase.Ended, "Auction not ended");
        require(randomEndBlockRequested, "Random end block not set");

        emit AuctionEnded(highestBidder, highestBid);
    }

    function withdraw() external {
        require(getCurrentPhase() == AuctionPhase.Ended, "Auction not ended");

        uint256 refundAmount = bids[msg.sender].amount;
        require(refundAmount > 0, "No refundable bid");
        require(msg.sender != highestBidder, "Winner cannot withdraw");

        bids[msg.sender].amount = 0;
        payable(msg.sender).transfer(refundAmount);
    }

    function getCurrentPhase() public view returns (AuctionPhase) {
        if (commitDeadline == 0) {
            return AuctionPhase.NotStarted;
        } else if (block.timestamp < commitDeadline) {
            return AuctionPhase.Commit;
        } else if (block.timestamp < revealDeadline) {
            return AuctionPhase.Reveal;
        } else {
            return AuctionPhase.Ended;
        }
    }

    function getAllBidders() external view returns (address[] memory) {
        return bidders;
    }

    function getCommitment(address user) external view returns (bytes32) {
        return bids[user].commitment;
    }

    function getRevealedBid(address user) external view returns (uint256) {
        return bids[user].amount;
    }

    function hasUserRevealed(address user) external view returns (bool) {
        return bids[user].revealed;
    }

    function getWinner() external view returns (address) {
        require(getCurrentPhase() == AuctionPhase.Ended, "Auction not ended");
        return highestBidder;
    }

    function getHighestBid() external view returns (uint256) {
        return highestBid;
    }

    function getHighestBidder() external view returns (address) {
        return highestBidder;
    }

    function getLastRequestId() external view returns (uint256) {
        return requestId;
    }
}
