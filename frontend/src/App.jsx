import { useEffect, useState } from "react";
import {
  BrowserProvider,
  Contract,
  keccak256,
  parseEther,
  AbiCoder,
  id,
  formatEther,
} from "ethers";
import CONTRACT_ABI from "../constants/abi.json";
import { CANDLE_AUCTION_ADDRESS } from "../constants/contractAddress";

export default function App() {
  const [provider, setProvider] = useState(null);
  const [signer, setSigner] = useState(null);
  const [contract, setContract] = useState(null);
  const [account, setAccount] = useState("");
  const [isOwner, setIsOwner] = useState(false);

  const [phase, setPhase] = useState(null);
  const [randomRequested, setRandomRequested] = useState(false);
  const [randomEndBlock, setRandomEndBlock] = useState(null);

  const [bidValue, setBidValue] = useState("");
  const [salt, setSalt] = useState("");
  const [loading, setLoading] = useState(false);

  const [revealedBids, setRevealedBids] = useState([]);
  const [winner, setWinner] = useState(null);
  const [highestBid, setHighestBid] = useState(null);
  const [settled, setSettled] = useState(false);

  // Connect wallet and init contract
  async function connectWallet() {
    if (!window.ethereum) {
      alert("Please install MetaMask");
      return;
    }

    try {
      const prov = new BrowserProvider(window.ethereum);
      const signer = await prov.getSigner();
      const addr = await signer.getAddress();
      const c = new Contract(CANDLE_AUCTION_ADDRESS, CONTRACT_ABI, signer);

      setProvider(prov);
      setSigner(signer);
      setAccount(addr);
      setContract(c);

      const owner = await c.owner();
      setIsOwner(owner.toLowerCase() === addr.toLowerCase());
    } catch (err) {
      console.error(err);
      alert("Connect failed: " + (err.reason || err.message));
    }
  }

  // Fetch current phase
  async function fetchPhase() {
    if (!contract) return;
    try {
      const p = await contract.getCurrentPhase();
      setPhase(Number(p));
    } catch (err) {
      console.error("fetchPhase:", err);
    }
  }

  // Fetch VRF status
  async function fetchRandom() {
    if (!contract) return;
    try {
      const req = await contract.randomEndBlockRequested();
      setRandomRequested(req);

      if (req) {
        const blockNum = await contract.randomEndBlock();
        setRandomEndBlock(blockNum);
      }
    } catch (err) {
      console.error("fetchRandom:", err);
    }
  }

  // Fetch revealed bids — use bigint comparison
  async function fetchRevealedBids() {
    if (!contract) return;
    try {
      const bidders = await contract.getAllBidders();
      const bids = await Promise.all(
        bidders.map(async (addr) => {
          const amount = await contract.getRevealedBid(addr);
          return { addr, amount };
        })
      );
      // filter out zero bids with bigint
      setRevealedBids(bids.filter((b) => b.amount > 0n));
    } catch (err) {
      console.error("fetchRevealedBids:", err);
    }
  }

  // Fetch winner & highest bid
  async function fetchWinner() {
    if (!contract) return;
    try {
      const winnerAddr = await contract.getHighestBidder();
      const bidAmt = await contract.getHighestBid();
      setWinner(winnerAddr);
      setHighestBid(bidAmt);
    } catch (err) {
      console.error("fetchWinner:", err);
    }
  }

  // Lifecycle: poll phase & random status
  useEffect(() => {
    if (!contract) return;
    fetchPhase();
    fetchRandom();

    const id1 = setInterval(fetchPhase, 5000);
    const id2 = setInterval(fetchRandom, 7000);
    return () => {
      clearInterval(id1);
      clearInterval(id2);
    };
  }, [contract]);

  // React to phase changes
  useEffect(() => {
    if (phase === 2) {
      fetchRevealedBids();
    }
    if (phase === 3) {
      fetchWinner();
    }
  }, [phase]);

  // Handlers
  async function startAuction() {
    setLoading(true);
    try {
      const tx = await contract.startAuction(120, 120);
      await tx.wait();
      alert("Auction started");
      fetchPhase();
    } catch (err) {
      console.error(err);
      alert("startAuction failed");
    } finally {
      setLoading(false);
    }
  }

  async function advancePhase() {
    setLoading(true);
    try {
      const tx = await contract.nextPhase();
      await tx.wait();
      alert("Phase advanced");
      fetchPhase();
    } catch (err) {
      console.error(err);
      alert("advancePhase failed");
    } finally {
      setLoading(false);
    }
  }

  async function requestRandomEnd() {
    setLoading(true);
    try {
      const tx = await contract.requestRandomEndBlock();
      await tx.wait();
      alert("Random request submitted");
      fetchRandom();
    } catch (err) {
      console.error(err);
      alert("requestRandomEndBlock failed");
    } finally {
      setLoading(false);
    }
  }

  async function commitBid() {
    if (!bidValue || !salt) {
      alert("Enter bid value and salt");
      return;
    }
    setLoading(true);
    try {
      const salt32 = id(salt);
      const parsed = parseEther(bidValue);
      const encoded = AbiCoder.defaultAbiCoder().encode(
        ["uint256", "bytes32"],
        [parsed, salt32]
      );
      const hash = keccak256(encoded);
      const tx = await contract.commitBid(hash, { value: parsed });
      await tx.wait();
      alert("Bid committed");
      setBidValue("");
      setSalt("");
    } catch (err) {
      console.error(err);
      alert("commitBid failed");
    } finally {
      setLoading(false);
    }
  }

  async function revealBid() {
    if (!bidValue || !salt) {
      alert("Enter bid value and salt");
      return;
    }
    setLoading(true);
    try {
      const salt32 = id(salt);
      const parsed = parseEther(bidValue);
      const tx = await contract.revealBid(parsed, salt32);
      await tx.wait();
      alert("Bid revealed");
      setBidValue("");
      setSalt("");
      fetchRevealedBids();
    } catch (err) {
      console.error(err);
      alert("revealBid failed");
    } finally {
      setLoading(false);
    }
  }

  async function settleAuction() {
    setLoading(true);
    try {
      const tx = await contract.settleAuction();
      await tx.wait();
      alert("Auction settled");
      setSettled(true);
      fetchWinner();
    } catch (err) {
      console.error(err);
      alert("settleAuction failed");
    } finally {
      setLoading(false);
    }
  }

  async function withdraw() {
    setLoading(true);
    try {
      const tx = await contract.withdraw();
      await tx.wait();
      alert("Withdrawn your deposit");
    } catch (err) {
      console.error(err);
      alert("withdraw failed");
    } finally {
      setLoading(false);
    }
  }

  // Render
  return (
    <main className="min-h-screen bg-gray-100 flex items-center justify-center p-4">
      <div className="bg-white shadow-xl rounded-2xl p-8 w-full max-w-lg">
        <h1 className="text-3xl font-bold mb-6 text-center">
          Candle Auction
        </h1>

        <button
          className="bg-blue-600 text-white px-4 py-2 rounded w-full mb-4"
          onClick={connectWallet}
        >
          {account
            ? `Connected: ${account.slice(0, 6)}...${account.slice(-4)}`
            : "Connect Wallet"}
        </button>

        <p className="mb-4 text-center">
          Phase:{" "}
          {["NotStarted", "Commit", "Reveal", "Ended"][phase] ||
            "Loading..."}
        </p>

        {isOwner && phase === 0 && (
          <button
            className="bg-green-600 text-white px-4 py-2 rounded w-full mb-4"
            onClick={startAuction}
            disabled={loading}
          >
            {loading ? "Starting..." : "Start Auction"}
          </button>
        )}

        {isOwner && phase > 0 && phase < 3 && (
          <button
            className="bg-gray-800 text-white px-4 py-2 rounded w-full mb-4"
            onClick={advancePhase}
            disabled={loading}
          >
            {loading ? "Advancing..." : "Advance Phase"}
          </button>
        )}

        <div className="space-y-4 mb-4">
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

          <button
            className="bg-purple-600 text-white px-4 py-2 rounded w-full"
            onClick={commitBid}
            disabled={loading || phase !== 1}
          >
            {loading && phase === 1 ? "..." : "Commit Bid"}
          </button>

          <button
            className="bg-yellow-600 text-white px-4 py-2 rounded w-full"
            onClick={revealBid}
            disabled={loading || phase !== 2}
          >
            {loading && phase === 2 ? "..." : "Reveal Bid"}
          </button>
        </div>

        {isOwner && phase === 2 && !randomRequested && (
          <button
            className="bg-indigo-600 text-white px-4 py-2 rounded w-full mb-4"
            onClick={requestRandomEnd}
            disabled={loading}
          >
            {loading ? "Requesting..." : "Request Random End Block"}
          </button>
        )}

        {phase === 2 && randomRequested && randomEndBlock && (
          <p className="text-center mb-4">
            Random end timestamp:{" "}
            {new Date(randomEndBlock * 1000).toLocaleString()}
          </p>
        )}

        {phase === 2 && revealedBids.length > 0 && (
          <div className="mt-6">
            <h2 className="text-xl font-semibold mb-2">Revealed Bids</h2>
            <ul className="space-y-1">
              {revealedBids.map((b, i) => (
                <li key={i} className="text-sm">
                  {b.addr.slice(0, 6)}…: {formatEther(b.amount)} ETH
                </li>
              ))}
            </ul>
          </div>
        )}

     

  

        {phase === 3 && (
          <div className="mt-6 text-center">
            {!settled && isOwner && (
              <button
                className="bg-red-600 text-white px-4 py-2 rounded mb-4"
                onClick={settleAuction}
                disabled={loading}
              >
                {loading ? "Settling..." : "Settle Auction"}
              </button>
            )}

            {winner && (
              <p className="text-lg mb-4">
                Winner: {winner.slice(0, 6)}… with{" "}
                {formatEther(highestBid)} ETH
              </p>
            )}

            {account.toLowerCase() !== winner?.toLowerCase() && (
              <button
                className="bg-gray-600 text-white px-4 py-2 rounded"
                onClick={withdraw}
                disabled={loading}
              >
                {loading ? "Withdrawing…" : "Withdraw Deposit"}
              </button>
            )}
          </div>
        )}
      </div>
    </main>
  );
}