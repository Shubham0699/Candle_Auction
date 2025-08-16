#!/usr/bin/env bash
RPC_URL="http://127.0.0.1:8545"
CONTRACT_ADDRESS="0xCE3478A9E0167a6Bc5716DC39DbbbfAc38F27623"

phase_name() {
  case $1 in
    0) echo NotStarted ;;
    1) echo Commit    ;;
    2) echo Reveal    ;;
    3) echo Ended     ;;
    *) echo Unknown   ;;
  esac
}

prev_phase=-1
while true; do
  raw=$(cast call $CONTRACT_ADDRESS "getCurrentPhase()(uint8)" --rpc-url $RPC_URL)
  phase=$(echo $raw | tr -d '\r')
  if [[ $phase != $prev_phase ]]; then
    echo "[$(date +'%T')] Phase changed → $(phase_name $phase)"
    prev_phase=$phase
    [[ $phase -eq 3 ]] && break  # stop when Ended
  fi
  sleep 2
done