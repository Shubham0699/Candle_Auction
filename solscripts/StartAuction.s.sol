// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import {CandleAuction} from "../src/CandleAuction.sol";

import "lib/forge-std/src/Script.sol";

contract StartAuctionScript is Script {
    function run() external {
        // load your deployer key
        uint256 deployerKey = vm.envUint("PRIVATE_KEY");

        // address of your already-deployed CandleAuction contract
        address auctionAddr = vm.envAddress("CONTRACT_ADDRESS");

        // define how long you want Commit & Reveal to last
        uint256 commitDuration = vm.envUint("COMMIT_DURATION"); // e.g. 300 seconds
        uint256 revealDuration = vm.envUint("REVEAL_DURATION"); // e.g. 300 seconds

        vm.startBroadcast(deployerKey);
        CandleAuction(auctionAddr).startAuction(commitDuration, revealDuration);
        vm.stopBroadcast();
    }
}
