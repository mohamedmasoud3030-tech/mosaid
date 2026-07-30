# Oracle Compute Instance — Provisioning Runbook

Status: **executable runbook, not yet executed.** No Oracle instance has been accessed from this repository and no credential has been used.

This runbook takes you from an empty Oracle Cloud tenancy to a host that is ready for `deploy/hermes/stage-release.sh`. It stops before staging Hermes, and it never starts the service.

Related: [`../hermes/README.md`](../hermes/README.md) · [`../../docs/pivot/ORACLE-DEPLOYMENT-PLAN.md`](../../docs/pivot/ORACLE-DEPLOYMENT-PLAN.md) · [`../../docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md`](../../docs/pivot/AGENTENV-EXECUTION-BACKEND-DECISION.md)

---

## 0. Ground rules

- The **Auth Tokens** page in the Oracle console is **not** the server and is **not** used for SSH. Never generate one for this deployment.
- Never paste an SSH private key, Auth Token, Telegram token or model key into chat, into Git, or into an issue.
- Secrets live only on the server, in files with mode `0600`, owned by `mosaid`.
- Nothing in this runbook installs Docker, AgentENV, `/dev/kvm` or any sandbox runtime. The first gate runs no code execution.

---

## 1. Choose the shape and confirm it is actually free

Oracle's Always Free Arm allocation has changed over time and is reported differently by different sources. **Do not assume.** Read the current quota in your own console before creating anything.

| Item | What to target | Where to confirm |
|---|---|---|
| Shape | `VM.Standard.A1.Flex` (Ampere, `aarch64`) | Create-instance page, "Always Free-eligible" badge |
| OCPU / RAM | Whatever your tenancy currently grants free — historically 4 OCPU / 24 GB, reported reduced to 2 OCPU / 12 GB in 2026 | Console quota + the Always Free badge |
| Boot volume | 50 GB (200 GB total across all instances) | Boot volume section |
| OS image | Ubuntu 24.04 LTS (preferred) or Oracle Linux 9 | Image selection |
| Region | The one with capacity; `ap-singapore-1` was observed on the owner's account | Region selector |

The `A1.Flex` ARM pool is frequently "out of capacity". If creation fails with `Out of host capacity`, retry in another availability domain or region, or retry later. Do not switch to a paid shape to work around it.

> **Billing gate.** If the "Always Free-eligible" badge is not shown on the exact configuration you are about to create, stop. Per Mosaid policy, unknown billing state is **denied**, and `OCI_BILLING_STATUS` stays `unknown` until you have visual confirmation.

An `VM.Standard.E2.1.Micro` (AMD, x86_64, 1 GB RAM) is also always free, but 1 GB RAM is tight for a Python agent runtime. Prefer A1.Flex; fall back to E2.1.Micro only for a smoke test.

---

## 2. Create the SSH key pair (on your own machine, not in the console)

```bash
ssh-keygen -t ed25519 -a 100 -f ~/.ssh/mosaid_oracle -C "mosaid-oracle"
```

- Set a passphrase.
- Upload **only** `~/.ssh/mosaid_oracle.pub` when creating the instance.
- `~/.ssh/mosaid_oracle` (the private key) never leaves your machine.

---

## 3. Create the instance

Console → Compute → Instances → **Create instance**:

1. **Name:** `mosaid-hermes`
2. **Image:** Ubuntu 24.04 LTS (or Oracle Linux 9)
3. **Shape:** `VM.Standard.A1.Flex`, then set OCPU/RAM within your free allocation
4. **Networking:** a VCN with a public subnet; assign a public IPv4 only if you need to reach it directly
5. **SSH keys:** paste the contents of `mosaid_oracle.pub`
6. **Boot volume:** 50 GB
7. Confirm the **Always Free-eligible** badge is visible, then Create

Record the public IP once the instance reaches **Running**.

---

## 4. Lock the network down to SSH only

Mosaid's first gate uses **outbound** Telegram long-polling. It needs **no inbound port except SSH**.

In the subnet's Security List / Network Security Group:

- **Ingress:** TCP `22` only. Restrict the source to your own IP (`x.x.x.x/32`) if it is stable; `0.0.0.0/0` is a last resort.
- **Do not** open `80`, `443`, `8000`, or any model/dashboard/sandbox port.
- **Egress:** allow all (needed for Telegram and the model endpoint).

Oracle images also ship a host firewall. Leave it enabled. Do not add rules.

---

## 5. First login and OS hardening

```bash
ssh -i ~/.ssh/mosaid_oracle ubuntu@<INSTANCE_IP>     # Oracle Linux uses: opc@<INSTANCE_IP>
```

Update and reboot:

```bash
sudo apt-get update && sudo apt-get -y upgrade && sudo reboot   # Ubuntu
# sudo dnf -y update && sudo reboot                             # Oracle Linux
```

Confirm password SSH is already disabled (Oracle images default to key-only):

```bash
sudo sshd -T | grep -Ei 'passwordauthentication|permitrootlogin|pubkeyauthentication'
```

Expected: `passwordauthentication no`, `permitrootlogin no` (or `prohibit-password`), `pubkeyauthentication yes`.

---

## 6. Collect the instance facts

Copy this repository to the instance (or `git clone` it), then run the read-only collector:

```bash
bash deploy/oracle/collect-instance-facts.sh
```

It writes a Markdown report to stdout and changes nothing. Paste the output into [`INSTANCE-FACTS.md`](INSTANCE-FACTS.md) and commit it — the script prints only non-secret facts.

This satisfies merge-gate items 3 and 4: OS, architecture, RAM, CPU, disk and billing metadata.

---

## 7. Prepare the host

```bash
sudo bash deploy/oracle/bootstrap-host.sh
```

Idempotent. It:

- verifies the OS is supported and `systemd` is present;
- installs `git`, `curl`, `ca-certificates` and a Python in the `>=3.11,<3.14` range;
- installs **`uv` pinned to `0.12.0`, verified by SHA-256** — downloaded to a file and checksummed, never piped into a shell;
- creates the `mosaid` system user with no login shell;
- creates `/opt/mosaid` and `/var/lib/mosaid` with correct ownership and modes;
- refuses to continue if Docker, AgentENV or a listening port `8000` is detected.

It does **not** clone Hermes, create secrets, or start anything.

---

## 8. Stage Hermes

Now the existing, already-reviewed path takes over:

```bash
python3 scripts/verify-hermes-pivot.py
sudo MOSAID_SOURCE="$PWD" bash deploy/hermes/stage-release.sh
```

Continue with [`../hermes/README.md`](../hermes/README.md) for secrets, preflight, first start and the acceptance gate.

---

## 9. What is still required from you

Non-secret facts, produced by step 6:

- instance state, public/private IP, OS image, architecture, OCPU, RAM, disk size, confirmed billing tier.

Secrets, created **on the server only** in step 8's `.env` at mode `0600`:

- Telegram bot token (from @BotFather), your numeric Telegram user ID, the model endpoint and its key.

Never send any of these in chat.
