---
name: blockchain-contract-attack
description: >-
  Blockchain / smart contracts: Etherscan, slither/mythril, reentrancy/access-control/oracle/flash-loan, cross-chain bridges, exposed RPC. Use when auditing smart contracts, DeFi, or blockchain attack surfaces.
metadata:
  tags: [penetration-testing, red-team]
---

## Blockchain / Smart Contracts

```
=== Blockchain / Smart Contracts ===
Source code: Etherscan getsourcecode API | Audit with slither/mythril/manticore
Manual review: Reentrancy (.call{value} transfers before state change, violating Checks-Effects-Interactions) | Access control (missing onlyOwner) | Integer overflow (<0.8 no SafeMath)
  Oracle manipulation (flash loan instantaneously manipulates AMM price) | Randomness (block.timestamp is controllable) | Authorization abuse (unlimited approve / permit replay) | delegatecall proxy storage collision
DeFi: Flash loan attacks / sandwich (front-running) / governance attacks / signature replay | Cross-chain bridges: signature threshold bypass / replay | RPC: exposed 8545 allows direct eth_sendTransaction
```
