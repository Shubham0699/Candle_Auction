// solscripts/RevealBid.s.sol
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.19;

import "../src/CandleAuction.sol";
import "lib/forge-std/src/Script.sol";

contract RevealBidScript is Script {
    function run() external {
        address auctionAddr = vm.envAddress("CONTRACT_ADDRESS");
        uint256 bidAmount = vm.envUint("BID_AMOUNT");
        uint256 rawSalt = vm.envUint("SALT");
        uint256 pk = vm.envUint("PRIVATE_KEY");

        CandleAuction auction = CandleAuction(payable(auctionAddr));

        // Cast the uint256 into bytes32 for your reveal call
        bytes32 salt = bytes32(rawSalt);

        vm.startBroadcast(pk);
        auction.revealBid(bidAmount, salt);
        vm.stopBroadcast();
    }
}
