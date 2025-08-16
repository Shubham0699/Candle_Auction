// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {VRFCoordinatorV2Interface} from
    "lib/chainlink-brownie-contracts/contracts/src/v0.8/vrf/interfaces/VRFCoordinatorV2Interface.sol";
import {VRFConsumerBaseV2} from "lib/chainlink-brownie-contracts/contracts/src/v0.8/vrf/VRFConsumerBaseV2.sol";

contract CandleAuction is Ownable, VRFConsumerBaseV2 {
    enum AuctionPhase {
        NotStarted,
        Commit,
        Reveal,
        Ended
    }

    AuctionPhase public phase;

    struct Bid {
        bytes32 commitment;
        uint256 amount;
        bool revealed;
    }

    // retained for VRF range/UI, but NOT used for gating
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
    event PhaseAdvanced(AuctionPhase newPhase);
    event BidCommitted(address indexed bidder);
    event BidRevealed(address indexed bidder, uint256 amount);
    event RandomEndBlockRequested();
    event RandomEndBlockFulfilled(uint256 blockNumber);
    event AuctionEnded(address winner, uint256 amount);

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

    modifier atPhase(AuctionPhase expected) {
        require(phase == expected, "Invalid auction phase");
        _;
    }

    /// @notice Kick things off: set deadlines (for VRF/UI) and enter Commit phase
    function startAuction(uint256 _commitDuration, uint256 _revealDuration)
        external
        onlyOwner
        atPhase(AuctionPhase.NotStarted)
    {
        commitDeadline = block.timestamp + _commitDuration;
        revealDeadline = commitDeadline + _revealDuration;
        phase = AuctionPhase.Commit;
        emit AuctionStarted(commitDeadline, revealDeadline);
    }

    /// @notice Manually advance: Commit → Reveal → Ended
    function nextPhase() external onlyOwner {
        if (phase == AuctionPhase.Commit) {
            phase = AuctionPhase.Reveal;
        } else if (phase == AuctionPhase.Reveal) {
            phase = AuctionPhase.Ended;
        } else {
            revert("No further phases");
        }
        emit PhaseAdvanced(phase);
    }

    /// @notice Place your deposit+hash in the Commit phase
    function commitBid(bytes32 hashedBid) external payable atPhase(AuctionPhase.Commit) {
        require(msg.value > 0, "Must send ETH with bid");
        require(bids[msg.sender].commitment == bytes32(0), "Already committed");

        bids[msg.sender] = Bid({commitment: hashedBid, amount: msg.value, revealed: false});

        bidders.push(msg.sender);
        emit BidCommitted(msg.sender);
    }

    /// @notice Reveal in the Reveal phase
    function revealBid(uint256 amount, bytes32 salt) external atPhase(AuctionPhase.Reveal) {
        Bid storage userBid = bids[msg.sender];
        require(userBid.commitment != bytes32(0), "No bid committed");
        require(!userBid.revealed, "Already revealed");

        bytes32 computedHash = keccak256(abi.encode(amount, salt));
        require(userBid.commitment == computedHash, "Commitment mismatch");
        require(userBid.amount == amount, "Reveal amount mismatch");

        userBid.revealed = true;
        revealedBids[msg.sender] = amount;

        // Only count if before randomEndBlock
        if (amount > highestBid && block.timestamp <= randomEndBlock) {
            highestBid = amount;
            highestBidder = msg.sender;
        }

        emit BidRevealed(msg.sender, amount);
    }

    /// @notice Request a random timestamp within [commitDeadline, revealDeadline)
    function requestRandomEndBlock() external onlyOwner atPhase(AuctionPhase.Reveal) {
        require(!randomEndBlockRequested, "Already requested");
        requestId =
            vrfCoordinator.requestRandomWords(keyHash, subscriptionId, requestConfirmations, callbackGasLimit, numWords);
        randomEndBlockRequested = true;
        emit RandomEndBlockRequested();
    }

    function fulfillRandomWords(uint256, uint256[] memory randomWords) internal override {
        require(randomEndBlockRequested, "Random not requested");
        uint256 range = revealDeadline - commitDeadline;
        uint256 offset = randomWords[0] % range;
        randomEndBlock = commitDeadline + offset;
        emit RandomEndBlockFulfilled(randomEndBlock);
    }

    /// @notice Emit the final winner in the Ended phase
    function settleAuction() external atPhase(AuctionPhase.Ended) {
        require(randomEndBlockRequested, "Random end block not set");
        emit AuctionEnded(highestBidder, highestBid);
    }

    /// @notice Refund losers (only in Ended)
    function withdraw() external atPhase(AuctionPhase.Ended) {
        uint256 refundAmount = bids[msg.sender].amount;
        require(refundAmount > 0, "No refundable bid");
        require(msg.sender != highestBidder, "Winner cannot withdraw");

        bids[msg.sender].amount = 0;
        payable(msg.sender).transfer(refundAmount);
    }

    /// @notice Now simply returns your manual phase
    function getCurrentPhase() public view returns (AuctionPhase) {
        return phase;
    }

    /// @notice Helpers
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
        require(phase == AuctionPhase.Ended, "Auction not ended");
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
