# Network layouts: same subnet, different subnets, several addresses

*Русская версия: [networks.ru.md](networks.ru.md)*

filedrop does **not** assume how your network is laid out. In the same room — and anywhere on the same site — a laptop and its panel can be:

| Layout | Example | What happens |
|---|---|---|
| **same subnet** | laptop `192.168.10.15/24`, panel `192.168.10.40/24` | the laptop talks to the panel directly |
| **different subnets** (routed) | laptop `172.26.1.86/16`, panel `10.173.42.104/16` | the traffic goes through the router; TCP 22 must be allowed between the subnets |
| **several addresses** | a laptop on cable *and* Wi-Fi, a panel with a second NIC | the address on a shared subnet is preferred |
| **mixed on one site** | some rooms in one subnet, some in two | every pair is judged on its own — no per-room or per-subnet settings |

The same `agent.conf` works for all of them.

## How the agent decides

1. **Every agent reports all its usable IPv4 addresses with their subnet masks** to the directory every `REGISTER_INTERVAL_SEC` seconds (`192.168.10.15/24`, …). Loopback, link-local and container/VM bridges (`docker0`, `br-*`, `veth*`, `virbr*` … see `IGNORE_INTERFACES`) are left out — they exist with identical addresses on many machines and would look like a "shared subnet". The directory also records the address the request came from.
2. **To send, the laptop asks the directory for the panel** and gets **all** the panel's addresses.
3. **It ranks them**: addresses inside one of the laptop's own subnets first ("same subnet"), then the others ("routed"). Its own addresses are never used.
4. **It connects to the first address whose SSH port answers** (3 s probe) and sends. If none answers, the item stays in the shared folder and the attempt is repeated every few seconds; the journal says which addresses failed.

The same ranking is applied to the answers of DNS (`dns` mode: all A records) and of the static table (`static` mode: several addresses per line), so the behaviour does not depend on the resolve mode.

The directory is only a phone book with a good memory of addresses; it never sees the files.

## Checking it

On a laptop:

```sh
filedrop-agent check
```

```
[OK  ] hostname pc-room12 -> role "pc", this machine is a client
      this machine's usable addresses: [172.26.1.86/16]
      counterpart candidates: [panel-room12]
[OK  ] panel-room12 resolves to [10.173.42.104] (directory mode)
[OK  ] 10.173.42.104 (other subnet, routed): port 22 is reachable
```

`same subnet` / `other subnet, routed` tells you which of the layouts above applies to this pair. In the directory's status page (`http://HOST:8781/status`, with the token header) you can see for each machine the address it connected from and all addresses it reported.

## Setting up the directory server for this

The server may sit in one subnet, or have an address in each (several NICs, or one box in a trunk/VLAN setup). Give **every** address of it in `DIRECTORY_URL` on **all** machines:

```
DIRECTORY_URL=http://10.0.0.5:8781, http://172.16.0.5:8781
```

An agent tries them in turn with a short timeout and then sticks to the one that worked, so the same line is valid in every subnet. (A one-time journal note appears when the first address was not the reachable one — harmless, reorder if you like.) Open TCP 8781 in the server's firewall from every subnet that has machines.

## Firewalls and routing between subnets

When the laptop and the panel are in different subnets, the router/firewall between them must allow **TCP 22 from the laptop's subnet to the panel's subnet** (replies are implicit). The panel never needs to connect to the laptop. In `dns` and `directory` modes `check` tells you immediately if the port is blocked.

## Typical situations

- **"In one room both are in the same subnet, in the next room they are not."** Nothing to do. Both rooms use the identical configuration.
- **A laptop moved between cable and Wi-Fi.** The next registration (≤ 45 s) updates the addresses; no restart.
- **The directory shows the wrong address for a host** (e.g. the router's address, because of NAT between the host and the directory). The reported addresses still contain the real ones, and the probe picks the address that answers.
- **No directory at all.** `dns` mode with a hosts/DNS entry having several A records, or `static` with several addresses per line, works the same way.
