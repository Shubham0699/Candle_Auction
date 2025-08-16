// solscripts/NextPhase.s.sol
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.19;

import "../src/CandleAuction.sol";
import "lib/forge-std/src/Script.sol";

contract NextPhaseScript is Script {
    function run() external {
        address auctionAddr = vm.envAddress("CONTRACT_ADDRESS");
        uint256 pk = vm.envUint("PRIVATE_KEY");

        CandleAuction auction = CandleAuction(payable(auctionAddr));

        vm.startBroadcast(pk);
        auction.nextPhase();
        vm.stopBroadcast();
    }
}
