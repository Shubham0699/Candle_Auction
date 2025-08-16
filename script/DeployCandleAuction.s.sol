// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "forge-std/Script.sol";
import "../src/CandleAuction.sol";

contract DeployCandleAuction is Script {
    function run() external returns (address) {
        // 1. Load your deployer key from .env
        uint256 deployerKey = vm.envUint("PRIVATE_KEY");
        vm.startBroadcast(deployerKey);

        // 2. Load VRF parameters from .env
        address vrfCoordinator = vm.envAddress("VRF_COORDINATOR_ADDRESS");
        uint64 subscriptionId = uint64(vm.envUint("VRF_SUBSCRIPTION_ID"));
        bytes32 keyHash = vm.envBytes32("VRF_KEY_HASH");
        uint32 callbackGasLimit = uint32(vm.envUint("VRF_CALLBACK_GAS"));
        uint16 requestConfirmations = uint16(vm.envUint("VRF_REQUEST_CONFS"));

        // 3. Deploy
        CandleAuction auction =
            new CandleAuction(vrfCoordinator, subscriptionId, keyHash, callbackGasLimit, requestConfirmations);

        vm.stopBroadcast();

        console.log(" Deployed CandleAuction at:", address(auction));
        return address(auction);
    }
}
