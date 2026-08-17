/** One TCP port a connected host was found listening on. */
export interface DiscoveredPort {
  /** Bind address as reported by the host: a literal IP, or '*' for a wildcard. */
  address: string
  port: number
  /** Program holding the socket; empty when the host could not attribute it. */
  process: string
  pid?: number
  /** Reachable only from the host itself, so a tunnel is the only way in. */
  loopback: boolean
  /** Best name we could establish for what is behind the port. */
  service: string
  /** Evidence behind `service`: a container image or the process command line. */
  detail: string
  /** Container publishing this port, when one does. */
  container: string
  /** How `service` was established. Only 'port' is guesswork. */
  origin: 'container' | 'process' | 'port' | ''
  /** 'http' or 'https' when this looks like a web UI worth opening as a panel. */
  scheme: string
}

/** An ad-hoc tunnel opened straight from a scan result. */
export interface DiscoveredForward {
  /** Negative: ad-hoc forwards have no persisted rule behind them. */
  ruleId: number
  /** What the tunnel actually bound locally, e.g. '127.0.0.1:43117'. */
  localAddr: string
  /** The remote port being forwarded. */
  port: number
}
