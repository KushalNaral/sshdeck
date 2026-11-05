import * as App from '../../wailsjs/go/main/App'

export interface CertInfo {
  subject: string
  issuer: string
  names: string[]
  notBefore: string
  notAfter: string
  expired: boolean
  selfSigned: boolean
  /** This computer already accepts the certificate, so trusting it explicitly is optional. */
  trustedBySystem: boolean
  /** Digest of the certificate (hex): what openfortivpn trusts. */
  sha256: string
  /** pin-sha256 of the public key: what openconnect trusts. */
  pin: string
  /** The digest in the colon-separated form administrators publish, for comparing by eye. */
  fingerprint: string
}

export interface CertApi {
  fetch(host: string, port: number): Promise<CertInfo>
}

const wailsCert: CertApi = {
  fetch: async (host, port) => (await App.FetchGatewayCert(host, port)) as unknown as CertInfo,
}

// Browser-only dev mode: a made-up self-signed certificate.
const mockCert: CertApi = {
  async fetch(host) {
    await new Promise((r) => setTimeout(r, 600))
    const hex = Array.from({ length: 32 }, (_, i) => ((i * 37 + 11) % 256).toString(16).padStart(2, '0')).join('')
    return {
      subject: `CN=${host || 'vpn.example.org'}`,
      issuer: `CN=${host || 'vpn.example.org'}`,
      names: [host || 'vpn.example.org'],
      notBefore: '2025-01-10T00:00:00Z',
      notAfter: '2027-01-10T00:00:00Z',
      expired: false,
      selfSigned: true,
      trustedBySystem: false,
      sha256: hex,
      pin: 'pin-sha256:' + 'A'.repeat(43) + '=',
      fingerprint: hex.match(/../g)!.join(':').toUpperCase(),
    }
  },
}

export const certApi: CertApi = 'go' in window ? wailsCert : mockCert
