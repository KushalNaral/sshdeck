# Organisation mode (optional, not built)

sshdeck is a personal tool by default: your own passphrase-locked vault, no server, no sign-in. **Nothing here is
required.** Organisation mode is an add-on for teams that want to separate concerns: one place that holds the server
list and credentials, and each person sees and uses only what they have been granted. An organisation that does not
want it never runs the central service, and the app never contacts one until a user chooses "Connect to an
organisation" and enters its address. It can be switched off again, which returns the app to the local vault.

## Goals
- A person signs in with the organisation's identity provider (OIDC) and receives only the servers and VPN profiles
  they are entitled to.
- The credential for a granted server is released when they connect, never displayed, and stops working when the
  grant is removed.
- A central record of who connected where, tied to the signed-in identity.
- Local servers and organisation servers can sit side by side; organisation ones are read-only and cannot be exported.

## Pieces
```
 sshdeck (desktop)  --OIDC auth code + PKCE-->  Identity provider (Keycloak first, any standard OIDC works)
        |                                                   |
        |  bearer token                                     | groups / roles in the token
        v                                                   v
   sshdeck server (Go, separate deployable)  <-- grants: group -> server / VPN profile
        |
        +-- inventory store (Postgres or SQLite)      credentials encrypted at rest (envelope: KMS or age key)
        +-- audit log keyed by the OIDC subject
```

**Sign-in.** The client opens the system browser for the OIDC authorization-code flow with PKCE and a loopback
redirect (`127.0.0.1`, random port). The organisation server's discovery document names the issuer and client id, so
the provider is the organisation's choice, not baked into the app. Tokens stay in memory (or inside the vault).

**API (all over TLS, bearer token):**
- `GET /v1/inventory`: the servers and VPN profiles the caller's groups or roles grant: id, name, host, port, user,
  VPN, production flag. No secrets.
- `POST /v1/servers/{id}/credential` and `/v1/vpns/{id}/credential`: checks the grant, writes a server-side audit
  entry, returns the secret once. The client holds it in memory for that connect only.
- `GET /v1/audit` for administrators.

**Grants.** A table of `group -> server | vpn profile`, edited by an administrator through a small admin UI or CLI.
Group names come from the token (Keycloak groups or roles). No per-user rows: membership changes in the identity
provider take effect at the next token refresh.

**Revocation.** Short access tokens (minutes) plus refresh tokens bound to the session. Removing someone from a group
stops new credential releases immediately. A credential already released stays valid until it is changed on the
target, so for shared passwords revocation means **rotating the password**; the service can track "needs rotation"
after a revoke.

## Honest limits
- A released password lives in the client's memory while it is in use. A determined user on their own machine can
  extract it. Organisation mode controls *who may obtain* a credential and records it; it cannot make a shared
  password unextractable.
- The central service becomes a high-value target: it holds every credential. It needs its own hardening, backups,
  key management and monitoring. That is the main cost of choosing this mode.

## Later phase: SSH certificates (no shared passwords)
After OIDC sign-in the service asks an SSH certificate authority to sign a short-lived (minutes) certificate for the
user's public key, restricted to the granted servers and a principal name. Servers trust the CA through
`TrustedUserCAKeys`. There is then nothing to extract and nothing to rotate, and the server's own logs name the person.
Prerequisite: every target must be configured to trust the CA. The same applies to VPNs where the gateway supports
certificate login.

## Where it plugs into the app
- **Credentials:** `App.serverSecret` (`conn.go`) is the one place a connect gets its secret. An organisation source
  returns the released secret there instead of reading the local store.
- **Inventory:** `App` reads servers, VPNs and tasks directly from `*store.Store`. When this is built, put a small
  interface (`Servers`, `VPNs`, `Tasks`, `Password`) in front of it and add an organisation implementation that merges
  the granted inventory into the list. It is not added now because nothing would use it yet.
- **Audit:** `audit.Entry.Actor` already exists; in organisation mode it carries the OIDC subject, and the same events
  are also sent to the server's log.
- **Lock:** the local vault still guards the cached token and local servers.

## Build order when it is wanted
1. The server with a static inventory and a token check against one provider.
2. Client sign-in and the merged server list (read-only organisation servers).
3. Credential release at connect, with server-side audit.
4. Grants admin and rotation tracking.
5. SSH certificate issuing, for servers that trust the CA.
