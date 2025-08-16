// solscripts/CommitBid.s.sol
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.19;

import "../src/CandleAuction.sol";
import "lib/forge-std/src/Script.sol";

//import "lib/forge-std/src/console.sol";

contract CommitBidScript is Script {
    function run() external {
        address auctionAddr = vm.envAddress("CONTRACT_ADDRESS");
        uint256 bidAmount = vm.envUint("BID_AMOUNT");
        uint256 salt = vm.envUint("SALT");
        uint256 deposit = vm.envUint("DEPOSIT");

        // Compute with encodePacked to match most auction implementations
        bytes32 sealedBid = keccak256(abi.encodePacked(bidAmount, salt));

        CandleAuction auction = CandleAuction(payable(auctionAddr));

        vm.startBroadcast();
        auction.commitBid{value: deposit}(sealedBid);
        vm.stopBroadcast();

        //console.log("  Sealed bid:", sealedBid);
        //console.log("  Deposit sent:", deposit);
    }
}
