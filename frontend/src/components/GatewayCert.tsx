import { useState } from 'react'
import { certApi, type CertInfo } from '../lib/cert'
import { messageOf } from '../lib/config'

interface Props {
  gateway: string
  port: number
  /** What the profile trusts now. */
  trustedCert?: string
  trustedPin?: string
  /** The user approved the certificate shown. */
  onTrust: (c: CertInfo) => void
  onClear: () => void
}

const day = (iso: string) => iso.slice(0, 10)

/**
 * Reads the certificate a VPN gateway presents and lets the user trust it, instead of failing a connect first and
 * pasting a hash from the error. The fingerprint is shown to compare with one from the gateway's administrator.
 */
export function GatewayCert({ gateway, port, trustedCert, trustedPin, onTrust, onClear }: Props) {
  const [info, setInfo] = useState<CertInfo | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const trusted = trustedCert || trustedPin

  const fetchIt = async () => {
    setBusy(true)
    setErr('')
    setInfo(null)
    try {
      setInfo(await certApi.fetch(gateway.trim(), port))
    } catch (e) {
      setErr(messageOf(e))
    }
    setBusy(false)
  }

  return (
    <fieldset className="logins-editor" aria-label="Gateway certificate">
      <legend>Gateway certificate</legend>
      {trusted ? (
        <p>
          Trusted: <code>{(trustedCert || trustedPin || '').slice(0, 30)}…</code>
        </p>
      ) : (
        <p className="dim">Only needed when the gateway uses a certificate this computer does not already trust, such as a self-signed one.</p>
      )}
      <div className="actions">
        <button className="btn" type="button" onClick={fetchIt} disabled={busy || !gateway.trim()}>
          {busy ? 'Reading…' : 'Fetch from gateway'}
        </button>
        {trusted && (
          <button className="btn" type="button" onClick={onClear}>
            Remove trusted certificate
          </button>
        )}
      </div>
      {err && (
        <p className="bad" role="alert">
          {err}
        </p>
      )}
      {info && (
        <div role="status">
          <dl className="cols">
            <dt>Subject</dt>
            <dd>{info.subject}</dd>
            <dt>Issuer</dt>
            <dd>{info.issuer}</dd>
            <dt>Valid</dt>
            <dd className={info.expired ? 'bad' : ''}>
              {day(info.notBefore)} to {day(info.notAfter)}
              {info.expired && ' (expired)'}
            </dd>
            <dt>Status</dt>
            <dd>
              {info.trustedBySystem
                ? 'Already trusted by this computer. Trusting it here also pins it.'
                : info.selfSigned
                  ? 'Self-signed: this computer does not trust it.'
                  : 'Not trusted by this computer.'}
            </dd>
            <dt>Fingerprint</dt>
            <dd>
              <code style={{ wordBreak: 'break-all' }}>{info.fingerprint}</code>
            </dd>
          </dl>
          <p className="dim">
            Compare the fingerprint with the one your VPN administrator gives you. If you cannot compare it, only trust it from a network you control: a
            certificate seen through someone else's connection could be theirs.
          </p>
          <div className="actions">
            <button className="btn solid" type="button" onClick={() => (onTrust(info), setInfo(null))}>
              Trust this certificate
            </button>
            <button className="btn" type="button" onClick={() => setInfo(null)}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </fieldset>
  )
}
