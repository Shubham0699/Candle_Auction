#!/usr/bin/env node
import { ethers } from "ethers";
import fs from "fs";
import dotenv from "dotenv";

dotenv.config();

const ABI = JSON.parse(fs.readFileSync("./constants/abi.json", "utf8"));
const { RPC_URL, PRIVATE_KEY, CONTRACT_ADDRESS } = process.env;

async function main() {
  const [bidValue, salt] = process.argv.slice(2);
  if (!bidValue || !salt) {
    console.error("Usage: commitBid.js <ETH_amount> <salt>");
    process.exit(1);
  }

  const provider = new ethers.JsonRpcProvider(RPC_URL);
  const wallet = new ethers.Wallet(PRIVATE_KEY, provider);
  const contract = new ethers.Contract(CONTRACT_ADDRESS, ABI, wallet);

  const parsedBid = ethers.parseEther(bidValue);
  const saltBytes = ethers.id(salt);
  const encoded = ethers.AbiCoder.defaultAbiCoder().encode(
    ["uint256", "bytes32"],
    [parsedBid, saltBytes]
  );
  const hash = ethers.keccak256(encoded);

  console.log(`Committing ${bidValue} ETH with hash ${hash}`);
  const tx = await contract.commitBid(hash, { value: parsedBid });
  console.log("Tx sent:", tx.hash);
  await tx.wait();
  console.log("✅ Bid committed!");
}

main().catch((e) => {
  console.error("Error:", e.message || e);
  process.exit(1);
});