# Candle Auction - Blockchain-based Commit-Reveal Auction with Chainlink VRF

A decentralized auction platform implementing a commit-reveal mechanism with a randomly determined end time using Chainlink VRF. Built with Foundry for smart contracts and React + Vite + Ethers.js for the optional frontend.

---

## Step 2: Project Setup

### Install Dependencies

```bash
git clone https://github.com/your-username/candle-auction.git
cd candle-auction
forge install
```

### Install Foundry (if not already installed)

```bash
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

### Project Structure

```
.
├── src/                  # Solidity contracts
├── script/              # Deployment scripts
├── test/                # Unit tests
├── frontend/            # React frontend (optional UI)
├── foundry.toml         # Foundry config
└── .env                 # Environment variables (RPC + Private key)
```

---

## Step 3: Contract Deployment

### Local Deployment (Anvil)

1. Start local node:

```bash
anvil
```

2. Create a `.env` file in the root with:

```env
RPC_URL=http://127.0.0.1:8545
PRIVATE_KEY=<your-private-key>
```

3. Deploy the contract using:

```bash
forge script script/DeployCandleAuction.s.sol --rpc-url $RPC_URL --private-key $PRIVATE_KEY --broadcast
```

> This will deploy the CandleAuction contract and log its address.

---

## Step 4: Testing the Contracts

Run unit tests using:

```bash
forge test -vvvv
```

* Tests include bid commitment, reveal mechanism, and VRF-based randomness.
* Edge cases like wrong salt, late reveals, and uncommitted addresses are handled.

---

## Step 5: Frontend (Optional UI)

### Stack

* Vite
* React
* Ethers.js v6
* TailwindCSS

### Run Frontend

```bash
cd frontend
npm install
npm run dev
```

> The UI is minimal and for demonstration purposes only. Core contract logic works independently of the frontend.

---

## Step 6: How It Works

### Commit Phase

* Users submit a hashed bid using `keccak256(amount, salt)`.
* Bid amount is locked in the contract.

### Reveal Phase

* Starts automatically after first bid with a timer.
* Bidders reveal their bid and salt.
* Highest valid reveal wins.

### Random End Time

* End of reveal phase is randomly selected using Chainlink VRF.
* Ensures unpredictability and fairness.

---

## Step 7: Key Features

* Fully decentralized commit-reveal logic
* Bid confidentiality using hashing + salt
* Uses Chainlink VRF for randomness
* Built with Foundry for development and testing
* Optional UI for MetaMask integration

---

## Future Improvements

* Upgrade frontend UI/UX
* Add support for Ethereum testnets/mainnet
* Improve error handling and bid history
* Add notifications and timers in frontend

---

## Author

**Shubh Bobade**
Reach out on [LinkedIn](https://www.linkedin.com/in/shubhambobade) | [GitHub](https://github.com/your-username)

---

## License

This project is licensed under the MIT License.
