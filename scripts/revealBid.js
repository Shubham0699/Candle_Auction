#!/usr/bin/env node
import { ethers } from "ethers";
import fs from "fs";
import dotenv from "dotenv";

dotenv.config();

const raw = fs.readFileSync("./constants/abi.json", "utf8");
let abi = JSON.parse(raw);
if (!Array.isArray(abi)) abi = abi.abi;

async function main() {
  const [bidValue, salt] = process.argv.slice(2);
  if (!bidValue || !salt) {
    console.error("Usage: revealBid.js <ETH_amount> <salt>");
    process.exit(1);
  }

  const provider = new ethers.JsonRpcProvider(process.env.RPC_URL);
  const wallet   = new ethers.Wallet(process.env.PRIVATE_KEY, provider);
  const contract = new ethers.Contract(process.env.CONTRACT_ADDRESS, abi, wallet);

  const parsed = ethers.parseEther(bidValue);
  const saltHash = ethers.id(salt); // matches commitBid.js

  console.log(`Revealing ${bidValue} ETH with salt hash ${saltHash}`);
  const tx = await contract.revealBid(parsed, saltHash, { gasLimit: 200_000 });
  console.log("Tx sent:", tx.hash);
  await tx.wait();
  console.log("✅ Bid revealed!");
}

main().catch((e) => {
  console.error("Error:", e.message || e);
  process.exit(1);
});