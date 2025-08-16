// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Test.sol";
import {CandleAuction} from "../src/CandleAuction.sol";
import {VRFCoordinatorV2Mock} from "./mocks/VRFCoordinatorV2Mock.sol";

contract CandleAuctionTest is Test {
    CandleAuction public auction;
    VRFCoordinatorV2Mock public vrfMock;

    address owner = address(0xABCD);
    address bidder1;
    address bidder2;

    bytes32 keyHash = bytes32("keyHash");
    uint64 subId = 1;
    uint32 callbackGasLimit = 500000;
    uint16 requestConfirmations = 3;
    uint96 baseFee = 0.25 ether;
    uint96 gasPriceLink = 1e9;

    function setUp() public {
        bidder1 = vm.addr(1);
        bidder2 = vm.addr(2);

        vrfMock = new VRFCoordinatorV2Mock(baseFee, gasPriceLink);
        vrfMock.createSubscription();
        vrfMock.fundSubscription(subId, 10 ether);

        vm.prank(owner);
        auction = new CandleAuction(
            address(vrfMock),
            subId,
            keyHash,
            callbackGasLimit,
            requestConfirmations
        );

        vrfMock.addConsumer(subId, address(auction));

        vm.deal(bidder1, 5 ether);
        vm.deal(bidder2, 5 ether);
    }

    function testFullAuctionFlow() public {
        // 1) Kick off auction → Commit phase
        vm.warp(100);
        vm.prank(owner);
        auction.startAuction(1000, 1000);

        // 2) Commit two bids
        bytes32 salt1 = bytes32("salt1___________________________");
        bytes32 salt2 = bytes32("salt2___________________________");
        bytes32 hash1 = keccak256(abi.encode(2 ether, salt1));
        bytes32 hash2 = keccak256(abi.encode(1 ether, salt2));

        vm.prank(bidder1);
        auction.commitBid{value: 2 ether}(hash1);
        vm.prank(bidder2);
        auction.commitBid{value: 1 ether}(hash2);

        assertEq(auction.getCommitment(bidder1), hash1);
        assertEq(auction.getCommitment(bidder2), hash2);

        // 3) Advance to Reveal phase
        vm.warp(auction.commitDeadline() + 1);
        vm.prank(owner);
        auction.nextPhase();

        // 4) Request & fulfill randomness
        vm.prank(owner);
        auction.requestRandomEndBlock();
        uint256 requestId = auction.getLastRequestId();
        vrfMock.fulfillRandomWords(requestId, address(auction));
        uint256 randomEnd = auction.randomEndBlock();
        assertGt(randomEnd, auction.commitDeadline());

        // 5) Reveal bids
        vm.prank(bidder1);
        auction.revealBid(2 ether, salt1);
        vm.prank(bidder2);
        auction.revealBid(1 ether, salt2);

        assertEq(auction.getRevealedBid(bidder1), 2 ether);
        assertEq(auction.getRevealedBid(bidder2), 1 ether);
        assertEq(auction.getHighestBidder(), bidder1);
        assertEq(auction.getHighestBid(), 2 ether);

        // 6) Advance to Ended phase
        vm.warp(auction.revealDeadline() + 1);
        vm.prank(owner);
        auction.nextPhase();

        // 7) Settle and verify winner
        vm.prank(bidder1);
        auction.settleAuction();
        assertEq(auction.getWinner(), bidder1);

        // 8) Withdraw refund for loser
        uint256 before = bidder2.balance;
        vm.prank(bidder2);
        auction.withdraw();
        uint256 afterBalance = bidder2.balance;
        assertGt(afterBalance, before);
    }

    /// @notice Reverts when the same bidder tries to commit twice
    function testDoubleCommitReverts() public {
        vm.warp(100);
        vm.prank(owner);
        auction.startAuction(1000, 1000);

        bytes32 hash = keccak256(abi.encode(1 ether, bytes32("salt")));
        vm.prank(bidder1);
        auction.commitBid{value: 1 ether}(hash);

        vm.prank(bidder1);
        vm.expectRevert("Already committed");
        auction.commitBid{value: 1 ether}(hash);
    }
}
