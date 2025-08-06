import { useEffect, useState } from "react";
import {
  BrowserProvider,
  Contract,
  keccak256,
  parseEther,
  AbiCoder,
  id,
} from "ethers";
import CONTRACT_ABI from "../constants/abi.json";
import { CANDLE_AUCTION_ADDRESS } from "../constants/contractAddress";

const CONTRACT_ADDRESS = CANDLE_AUCTION_ADDRESS;

export default function App() {
  const [provider, setProvider] = useState(null);
  const [signer, setSigner] = useState(null);
  const [contract, setContract] = useState(null);
  const [account, setAccount] = useState("");
  const [isOwner, setIsOwner] = useState(false);
  const [phase, setPhase] = useState(null);
  const [bidValue, setBidValue] = useState("");
  const [salt, setSalt] = useState("");
  const [loading, setLoading] = useState(false);

  // Connect Wallet
  async function connectWallet() {
  if (!window.ethereum) return alert("Install MetaMask to use this app");

  try {
    const prov = new BrowserProvider(window.ethereum);
    const signer = await prov.getSigner();
    const address = await signer.getAddress();
    const candleContract = new Contract(CONTRACT_ADDRESS, CONTRACT_ABI, signer);

    setProvider(prov);
    setSigner(signer);
    setAccount(address);
    setContract(candleContract);

    // 🔐 Wait for block 4+ to avoid "BlockOutOfRangeError"
    let latestBlock = await prov.getBlockNumber();
    console.log("Waiting for block >= 4 (current:", latestBlock, ")");

    while (latestBlock < 4) {
      await new Promise((res) => setTimeout(res, 1000));
      latestBlock = await prov.getBlockNumber();
    }

    // 🔐 Try owner read after enough blocks
    const owner = await candleContract.owner();
    console.log("Contract owner:", owner);
    setIsOwner(owner.toLowerCase() === address.toLowerCase());
  } catch (err) {
    console.error("connectWallet failed:", err);
    alert("Wallet connect failed: " + (err.reason || err.message));
    setIsOwner(false);
  }
}


  // Get Auction Phase
  async function fetchPhase() {
    if (!contract) return;
    const phase = await contract.getCurrentPhase();
    setPhase(Number(phase));
  }

  useEffect(() => {
    if (contract) {
      fetchPhase();
      const interval = setInterval(() => {
        fetchPhase();
      }, 5000);
      return () => clearInterval(interval);
    }
  }, [contract]);

  // Start Auction (owner only)
  async function startAuction() {
    try {
      setLoading(true);
      const tx = await contract.startAuction(120, 120); // 2 min commit & reveal
      await tx.wait();
      alert("Auction started.");
    } catch (err) {
      alert("Failed to start auction.");
      console.error(err);
    } finally {
      setLoading(false);
    }
  }

  // Commit Bid
  async function commitBid() {
    if (!bidValue || !salt) return alert("Enter bid value and salt.");
    try {
      setLoading(true);
      const saltBytes32 = id(salt);
      const parsedBid = parseEther(bidValue);
      const encoded = AbiCoder.defaultAbiCoder().encode(
        ["uint256", "bytes32"],
        [parsedBid, saltBytes32]
      );
      const hashed = keccak256(encoded);

      const tx = await contract.commitBid(hashed, {
        value: parsedBid,
      });

      await tx.wait();
      alert("Bid committed!");
    } catch (err) {
      alert("Commit failed.");
      console.error(err);
    } finally {
      setLoading(false);
    }
  }

  // Reveal Bid
  async function revealBid() {
    if (!bidValue || !salt) return alert("Enter bid value and salt.");
    try {
      setLoading(true);
      const saltBytes32 = id(salt);
      const parsedBid = parseEther(bidValue);
      const tx = await contract.revealBid(parsedBid, saltBytes32);
      await tx.wait();
      alert("Bid revealed!");
    } catch (err) {
      alert("Reveal failed.");
      console.error(err);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="min-h-screen bg-gray-100 flex flex-col items-center justify-center p-4">
      <div className="bg-white shadow-xl rounded-2xl p-8 w-full max-w-lg">
        <h1 className="text-3xl font-bold mb-6 text-center">Candle Auction</h1>

        <button
          className="bg-blue-600 text-white px-4 py-2 rounded w-full mb-4"
          onClick={connectWallet}
        >
          {account
            ? `Connected: ${account.slice(0, 6)}...${account.slice(-4)}`
            : "Connect Wallet"}
        </button>

        <p className="mb-2 text-center">
          <strong>Phase:</strong>{" "}
          {["Not Started", "Commit", "Reveal", "Ended"][phase] || "Loading..."}
        </p>

        {isOwner && phase === 0 && (
          <button
            className="bg-green-600 text-white px-4 py-2 rounded w-full mb-4"
            onClick={startAuction}
            disabled={loading}
          >
            {loading ? "Starting..." : "Start Auction (Owner)"}
          </button>
        )}

        <div className="space-y-4">
          <input
            className="w-full border p-2 rounded"
            type="text"
            placeholder="Bid Amount (ETH)"
            value={bidValue}
            onChange={(e) => setBidValue(e.target.value)}
          />

          <input
            className="w-full border p-2 rounded"
            type="text"
            placeholder="Secret Salt"
            value={salt}
            onChange={(e) => setSalt(e.target.value)}
          />

          {phase === 1 && (
            <button
              className="bg-purple-600 text-white px-4 py-2 rounded w-full"
              onClick={commitBid}
              disabled={loading}
            >
              {loading ? "Committing..." : "Commit Bid"}
            </button>
          )}

          {phase === 2 && (
            <button
              className="bg-yellow-600 text-white px-4 py-2 rounded w-full"
              onClick={revealBid}
              disabled={loading}
            >
              {loading ? "Revealing..." : "Reveal Bid"}
            </button>
          )}
        </div>
      </div>
    </main>
  );
}
