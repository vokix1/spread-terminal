import { useEffect, useMemo, useState } from 'react'

type MarketEvent = {
  exchange: string
  symbol: string
  bid: number
  ask: number
  bidQty: number
  askQty: number
  time: string
}

type Tab = 'opportunities' | 'coin'
type RouteType = 'all' | 'spot-spot' | 'spot-future' | 'future-future'

const API_WS = import.meta.env.VITE_MARKET_WS ?? 'ws://127.0.0.1:8080/ws/market'

function formatNumber(value: number, digits = 4) {
  return new Intl.NumberFormat('en-US', { maximumFractionDigits: digits }).format(value)
}

function App() {
  const [tab, setTab] = useState<Tab>('opportunities')
  const [status, setStatus] = useState<'connecting' | 'live' | 'offline'>('connecting')
  const [quotes, setQuotes] = useState<Record<string, MarketEvent>>({})
  const [query, setQuery] = useState('')
  const [routeType, setRouteType] = useState<RouteType>('all')
  const [sortKey, setSortKey] = useState<'spread' | 'liquidity' | 'symbol'>('spread')
  const [selected, setSelected] = useState('BTCUSDT')

  useEffect(() => {
    let socket: WebSocket | null = null
    let retryTimer: number | undefined
    let stopped = false

    const connect = () => {
      setStatus('connecting')
      socket = new WebSocket(API_WS)
      socket.onopen = () => setStatus('live')
      socket.onmessage = (message) => {
        const event = JSON.parse(message.data) as MarketEvent
        setQuotes((current) => ({ ...current, [event.symbol]: event }))
      }
      socket.onerror = () => socket?.close()
      socket.onclose = () => {
        if (stopped) return
        setStatus('offline')
        retryTimer = window.setTimeout(connect, 2000)
      }
    }

    connect()
    return () => {
      stopped = true
      if (retryTimer) window.clearTimeout(retryTimer)
      socket?.close()
    }
  }, [])

  const rows = useMemo(() => {
    const normalized = Object.values(quotes)
      .filter((quote) => quote.symbol.endsWith('USDT'))
      .map((quote) => ({
        ...quote,
        spread: quote.ask > 0 ? ((quote.ask - quote.bid) / quote.ask) * 100 : 0,
        liquidity: quote.bid * quote.bidQty + quote.ask * quote.askQty,
        route: 'spot-spot' as const,
      }))
      .filter((quote) => quote.symbol.toLowerCase().includes(query.toLowerCase()))
      .filter((quote) => routeType === 'all' || quote.route === routeType)

    return normalized.sort((a, b) => {
      if (sortKey === 'symbol') return a.symbol.localeCompare(b.symbol)
      return b[sortKey] - a[sortKey]
    })
  }, [quotes, query, routeType, sortKey])

  const selectedQuote = quotes[selected] ?? rows[0]

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ST</div>
          <div><strong>Spread Terminal</strong><span>Market intelligence</span></div>
        </div>
        <nav>
          <button className={tab === 'opportunities' ? 'active' : ''} onClick={() => setTab('opportunities')}>Opportunities</button>
          <button className={tab === 'coin' ? 'active' : ''} onClick={() => setTab('coin')}>Coin analytics</button>
          <button disabled>Funding scanner</button>
          <button disabled>Execution</button>
          <button disabled>System</button>
        </nav>
        <div className="connection-card">
          <span className={`status-dot ${status}`} />
          <div><strong>{status === 'live' ? 'Live market feed' : status}</strong><span>Binance book ticker</span></div>
        </div>
      </aside>

      <main>
        <header className="topbar">
          <div><span className="eyebrow">Crypto arbitrage workspace</span><h1>{tab === 'opportunities' ? 'All opportunities' : `${selected.replace('USDT', '')} analytics`}</h1></div>
          <div className="live-pill"><span className={`status-dot ${status}`} />{Object.keys(quotes).length} live quotes</div>
        </header>

        {tab === 'opportunities' ? (
          <>
            <section className="metrics">
              <Metric label="Tracked markets" value={String(Object.keys(quotes).length)} detail="Live top-of-book" />
              <Metric label="Visible USDT pairs" value={String(rows.length)} detail="Filtered universe" />
              <Metric label="Best spread" value={rows[0] ? `${rows[0].spread.toFixed(4)}%` : '—'} detail={rows[0]?.symbol ?? 'Waiting for data'} />
              <Metric label="Feed status" value={status.toUpperCase()} detail={API_WS} />
            </section>

            <section className="panel">
              <div className="filters">
                <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search BTC, ETH, SOL…" />
                <select value={routeType} onChange={(event) => setRouteType(event.target.value as RouteType)}>
                  <option value="all">All routes</option>
                  <option value="spot-spot">Spot ↔ Spot</option>
                  <option value="spot-future" disabled>Spot ↔ Future</option>
                  <option value="future-future" disabled>Future ↔ Future</option>
                </select>
                <select value={sortKey} onChange={(event) => setSortKey(event.target.value as typeof sortKey)}>
                  <option value="spread">Sort by spread</option>
                  <option value="liquidity">Sort by top liquidity</option>
                  <option value="symbol">Sort by symbol</option>
                </select>
              </div>

              <div className="table-wrap">
                <table>
                  <thead><tr><th>Coin</th><th>Route</th><th>Bid</th><th>Ask</th><th>Spread</th><th>Top liquidity</th><th>Updated</th></tr></thead>
                  <tbody>
                    {rows.slice(0, 200).map((row) => (
                      <tr key={row.symbol} onClick={() => { setSelected(row.symbol); setTab('coin') }}>
                        <td><strong>{row.symbol.replace('USDT', '')}</strong><span className="sub">{row.symbol}</span></td>
                        <td><span className="route-badge">Binance spot</span></td>
                        <td>{formatNumber(row.bid, 8)}</td>
                        <td>{formatNumber(row.ask, 8)}</td>
                        <td className="positive">{row.spread.toFixed(5)}%</td>
                        <td>${formatNumber(row.liquidity, 0)}</td>
                        <td>{row.time ? new Date(row.time).toLocaleTimeString() : 'live'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {rows.length === 0 && <div className="empty">Waiting for live Binance quotes… Keep the Go backend running on port 8080.</div>}
              </div>
            </section>
          </>
        ) : (
          <CoinAnalytics quote={selectedQuote} onBack={() => setTab('opportunities')} />
        )}
      </main>
    </div>
  )
}

function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <div className="metric"><span>{label}</span><strong>{value}</strong><small>{detail}</small></div>
}

function CoinAnalytics({ quote, onBack }: { quote?: MarketEvent; onBack: () => void }) {
  if (!quote) return <section className="panel empty">No quote selected yet.</section>
  const spread = quote.ask > 0 ? ((quote.ask - quote.bid) / quote.ask) * 100 : 0
  const mid = (quote.bid + quote.ask) / 2
  const points = Array.from({ length: 28 }, (_, index) => mid * (1 + Math.sin(index / 3) * 0.0008 + index * 0.00001))
  const min = Math.min(...points)
  const max = Math.max(...points)
  const path = points.map((point, index) => `${(index / (points.length - 1)) * 100},${92 - ((point - min) / (max - min || 1)) * 80}`).join(' ')

  return <>
    <button className="back" onClick={onBack}>← Back to opportunities</button>
    <section className="coin-grid">
      <div className="panel chart-panel">
        <div className="panel-title"><div><span className="eyebrow">Price overview</span><h2>{quote.symbol}</h2></div><strong>${formatNumber(mid, 8)}</strong></div>
        <svg className="chart" viewBox="0 0 100 100" preserveAspectRatio="none"><polyline points={path} fill="none" stroke="currentColor" strokeWidth="2" vectorEffect="non-scaling-stroke" /></svg>
      </div>
      <div className="panel stats-panel">
        <h2>Market details</h2>
        <Detail label="Exchange" value={quote.exchange} />
        <Detail label="Best bid" value={formatNumber(quote.bid, 8)} />
        <Detail label="Best ask" value={formatNumber(quote.ask, 8)} />
        <Detail label="Spread" value={`${spread.toFixed(5)}%`} />
        <Detail label="Bid quantity" value={formatNumber(quote.bidQty, 6)} />
        <Detail label="Ask quantity" value={formatNumber(quote.askQty, 6)} />
      </div>
    </section>
    <section className="analytics-grid">
      <AnalyticsCard title="Funding" value="Awaiting futures adapter" detail="Will compare rates across Binance, OKX, Bybit and Hyperliquid." />
      <AnalyticsCard title="Open interest" value="Awaiting OI feed" detail="Current OI, 1h/24h change and exchange distribution." />
      <AnalyticsCard title="Basis" value="Awaiting spot-perp routes" detail="Spot/perpetual premium and annualized carry." />
      <AnalyticsCard title="Executable spread" value={`${spread.toFixed(5)}%`} detail="Currently top-of-book only; depth/VWAP validation comes next." />
    </section>
  </>
}

function Detail({ label, value }: { label: string; value: string }) { return <div className="detail"><span>{label}</span><strong>{value}</strong></div> }
function AnalyticsCard({ title, value, detail }: { title: string; value: string; detail: string }) { return <div className="panel analytics-card"><span>{title}</span><strong>{value}</strong><p>{detail}</p></div> }

export default App
