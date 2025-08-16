// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";

// ← Adjusted import path to your local brownie package
import "chainlink-brownie-contracts/contracts/src/v0.8/vrf/mocks/VRFCoordinatorV2Mock.sol";

import "../src/CandleAuction.sol";

contract DeployMocksAndAuction is Script {
    function run() external returns (address auctionAddr) {
        uint256 deployerKey = vm.envUint("PRIVATE_KEY");
        vm.startBroadcast(deployerKey);

        VRFCoordinatorV2Mock vrfMock = new VRFCoordinatorV2Mock(
            /* baseFee */
            0,
            /* gasPriceLink */
            0
        );

        uint64 subId = vrfMock.createSubscription();
        vrfMock.fundSubscription(subId, 1e18);

        CandleAuction auction = new CandleAuction(address(vrfMock), subId, bytes32(0), uint32(200_000), uint16(3));

        vm.stopBroadcast();

        console.log("VRF Mock at:", address(vrfMock));
        console.log("Auction at:", address(auction));

        return address(auction);
    }
}
