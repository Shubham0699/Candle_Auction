# Candle Auction – Commit-Reveal Auction with Chainlink VRF

A decentralized auction platform implementing a commit-reveal mechanism with a randomly determined end time using Chainlink VRF. Built with Foundry for smart contracts and React + Vite + Ethers.js for the optional frontend.

---

## Step 1: Project Setup

### Clone & Install

```bash
git clone https://github.com/your-username/candle-auction.git
cd candle-auction

# Install Foundry dependencies
forge install

# Install frontend dependencies
cd frontend
npm install
cd ..
```

### Environment Variables

Create a `.env` in the root:

```env
SEPOLIA_RPC_URL=<your Sepolia RPC URL>
PRIVATE_KEY=<deployer private key>
VRF_SUBSCRIPTION_ID=<Chainlink VRF v2 subscription ID>
LINK_TOKEN_ADDRESS=<>
VRF_COORDINATOR_ADDRESS=<>
```

---

## Step 2: Project Structure

```
.
├── src/
│   └── CandleAuction.sol               # Modified main contract
├── solscripts/                         # Forge scripts for manual interactions
│   ├── CommitBid.s.sol
│   ├── RevealBid.s.sol
│   └── NextPhase.s.sol
|   |__StartAuction.s.sol
├── script/                             # Deployment scripts (Solidity)
│   ├── DeployCandleAuction.s.sol       # Deploy core contract
│   └── DeployMocksAndAuction.s.sol     # Deploy mocks + full auction locally
├── scripts/                            # JS scripts for automation
│   ├── commit.js                       # Commit phase via ethers.js
│   └── reveal.js                       # Reveal phase via ethers.js
├── test/
│   └── CandleAuctionTest.t.sol         # Expanded unit tests
├── frontend/
│   └── App.jsx                         # Modified React frontend
├── foundry.toml
└── .env
```

---

## Step 3: Contract Deployment

### Local (Anvil)

1. Start local node:

   ```bash
   anvil
   ```

2. Deploy mocks and auction:

   ```bash
   forge script \
     script/DeployMocksAndAuction.s.sol \
     --rpc-url http://127.0.0.1:8545 \
     --private-key $PRIVATE_KEY \
     --broadcast
   ```

### Sepolia Testnet

Deploy core contract only:

```bash
forge script \
  script/DeployCandleAuction.s.sol:DeployCandleAuction \
  --rpc-url $SEPOLIA_RPC_URL \
  --private-key $PRIVATE_KEY \
  --broadcast
```

After deployment, update your frontend’s contract address if needed.

---

## Step 4: Automated Interactions

### Forge “solscripts”

- Commit phase:

  ```bash
  forge script \
    solscripts/CommitBid.s.sol:CommitBid \
    --rpc-url $SEPOLIA_RPC_URL \
    --private-key $PRIVATE_KEY \
    --broadcast \
    --args <contract> <bidHash> <value>
  ```

- Reveal phase:

  ```bash
  forge script \
    solscripts/RevealBid.s.sol:RevealBid \
    --rpc-url $SEPOLIA_RPC_URL \
    --private-key $PRIVATE_KEY \
    --broadcast \
    --args <contract> <amount> <salt>
  ```

- Advance phase:

  ```bash
  forge script \
    solscripts/NextPhase.s.sol:NextPhase \
    --rpc-url $SEPOLIA_RPC_URL \
    --private-key $PRIVATE_KEY \
    --broadcast \
    --args <contract>
  ```

### JS Automation

In the root, run:

```bash
node scripts/commit.js \
  --contract 0xYourAuctionAddress \
  --amount 1.0 \
  --salt 0xabc123 \
  --rpc $SEPOLIA_RPC_URL \
  --key $PRIVATE_KEY

node scripts/reveal.js \
  --contract 0xYourAuctionAddress \
  --amount 1.0 \
  --salt 0xabc123 \
  --rpc $SEPOLIA_RPC_URL \
  --key $PRIVATE_KEY
```

These helpers wrap the ethers.js calls for commit and reveal.

---

## Step 5: Testing

Run all unit tests (including new edge-case tests) via Foundry:

```bash
forge test -vvvv
```

Tests cover:
- Bid commitment + reveal
- Incorrect salt or amount
- Phase gating
- VRF-driven random end timestamp

---

## Step 6: Frontend (Optional UI)

### Run the frontend

```bash
cd frontend
npm run dev
```

Connect MetaMask to Sepolia, input your contract address, and you’ll see buttons for:
- startAuction
- commitBid
- nextPhase
- revealBid
- requestRandomEndBlock
- settleAuction
- withdraw

---

## How It Works

1. **Commit Phase**  
   Users submit `keccak256(amount, salt)` and lock ETH.

2. **Reveal Phase**  
   Bidders open their bids with `(amount, salt)`.  
   Highest valid bid before the random cutoff wins.

3. **Random End**  
   Owner calls `requestRandomEndBlock()`.  
   Chainlink VRF supplies a random timestamp between deadlines.

4. **Settlement**  
   After final phase, owner calls `settleAuction()` and bidders withdraw.

---

## Key Features

- Decentralized commit-reveal auction  
- Confidential bids via hashing & salt  
- Randomized reveal cutoff with Chainlink VRF  
- Modular Forge scripts and JS automation  
- Expanded unit tests for edge cases  
- Minimal React UI for end-to-end demo

---

## Future Improvements

- CI integration for automated deploy + tests  
- Enhanced frontend UX (timers, notifications)  
- Gas-optimizations in scripts  
- Mainnet deployment support

---

## Author

**Shubh Bobade**
Reach out on [LinkedIn](linkedin.com/in/shubham-bobade-b8432a246) | [GitHub](https://github.com/Shubham0699)

---

## License

This project is licensed under the MIT License.
