// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/console.sol";
import {Script} from "forge-std/Script.sol";
import {CandleAuction} from "../src/CandleAuction.sol";

contract DeployCandleAuction is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address initialOwner = vm.addr(deployerPrivateKey);

        address vrfCoordinator = 0x0000000000000000000000000000000000000000;
        uint64 subscriptionId = 0;
        bytes32 keyHash = 0x0000000000000000000000000000000000000000000000000000000000000000;
        uint32 callbackGasLimit = 100000;
        uint16 requestConfirmations = 3;

        vm.startBroadcast(deployerPrivateKey);

        CandleAuction candleAuction = new CandleAuction(
            vrfCoordinator,
            subscriptionId,
            keyHash,
            callbackGasLimit,
            requestConfirmations
        );

        uint biddingTime = 300;
        uint revealTime = 180;
        candleAuction.startAuction(biddingTime, revealTime);

        console.log("CandleAuction deployed at:", address(candleAuction));
        console.log("Owner address:", candleAuction.owner());

        vm.stopBroadcast();
    }
}
