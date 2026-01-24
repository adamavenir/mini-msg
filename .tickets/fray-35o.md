---
id: fray-35o
status: closed
deps: []
links: []
created: 2025-12-22T11:44:54.024023-08:00
type: feature
priority: 2
---
# Add claims section to quickstart

Add COLLISION PREVENTION section to quickstart covering:

  mm claim @agent --file src/auth.ts      Claim a file
  mm claim @agent --files "*.ts,*.go"     Claim multiple patterns
  mm claim @agent --file x --ttl 2h       Claim with expiration
  mm claims                               List all active claims
  mm claims @agent                        List agent's claims  
  mm clear @agent                         Clear all your claims
  mm clear @agent --file src/auth.ts      Clear specific claim

Explain: prevents other agents from accidentally working on same files. Claims auto-clear on mm bye.


