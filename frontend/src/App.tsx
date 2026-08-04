import { useEffect, useMemo, useRef, useState } from 'react'

// ── Types ──────────────────────────────────────────────────────────────────────
type MarketType = 'spot' | 'perp' | 'futures'

type Opportunity = {
  base: string
  buyExchange: string
  buySymbol: string
  buyType: MarketType
  buyAsk: number
  sellExchange: string
  sellSymbol: string
  sellType: MarketType
  sellBid: number
  spreadPct: number
  volume24h: number
  openInterest: number
  fundingRate: number
  score: number
  updatedAt: string
}

type OppKey = string // `${base}:${buyExchange}:${buySymbol}:${sellExchange}:${sellSymbol}`

type Tab = 'opportunities' | 'coin'
type SortKey = 'score' | 'spreadPct' | 'volume24h'

const API_WS = import.meta.env.VITE_MARKET_WS ?? 'ws://127.0.0.1:8080/ws/market'

function oppKey(o: Opportunity): OppKey {
  return `${o.base}:${o.buyExchange}:${o.buySymbol}:${o.sellExchange}:${o.sellSymbol}`
}

function fmt(value: number, digits = 4): string {
  return new Intl.NumberFormat('ru-RU', { maximumFractionDigits: digits }).format(value)
}

function fmtUSD(value: number): string {
  if (value >= 1e9) return `$${(value / 1e9).toFixed(2)}B`
  if (value >= 1e6) return `$${(value / 1e6).toFixed(2)}M`
  if (value >= 1e3) return `$${(value / 1e3).toFixed(1)}K`
  return `$${fmt(value, 2)}`
}

function routeLabel(o: Opportunity): string {
  const types: Record<MarketType, string> = { spot: 'Спот', perp: 'Перп', futures: 'Фьюч' }
  return `${types[o.buyType] ?? o.buyType} ↔ ${types[o.sellType] ?? o.sellType}`
}

// ── App ────────────────────────────────────────────────────────────────────────
export default function App() {
  const [tab, setTab] = useState<Tab>('opportunities')
  const [status, setStatus] = useState<'connecting' | 'live' | 'offline'>('connecting')
  const [opps, setOpps] = useState<Map<OppKey, Opportunity>>(new Map())
  const [query, setQuery] = useState('')
  const [sortKey, setSortKey] = useState<SortKey>('score')
  const [selected, setSelected] = useState<Opportunity | null>(null)

  // ── WebSocket ────────────────────────────────────────────────────────────
  useEffect(() => {
    let socket: WebSocket | null = null
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let stopped = false

    const connect = () => {
      setStatus('connecting')
      socket = new WebSocket(API_WS)
      socket.onopen = () => setStatus('live')
      socket.onmessage = (ev) => {
        try {
          const opp = JSON.parse(ev.data) as Opportunity
          if (!opp.base || !opp.buyAsk || !opp.sellBid) return
          setOpps((prev) => {
            const next = new Map(prev)
            next.set(oppKey(opp), opp)
            return next
          })
        } catch { /* ignore parse errors */ }
      }
      socket.onerror = () => socket?.close()
      socket.onclose = () => {
        if (stopped) return
        setStatus('offline')
        retryTimer = setTimeout(connect, 2000)
      }
    }

    connect()
    return () => {
      stopped = true
      clearTimeout(retryTimer)
      socket?.close()
    }
  }, [])

  // ── Derived data ─────────────────────────────────────────────────────────
  const rows = useMemo(() => {
    const q = query.toLowerCase()
    return Array.from(opps.values())
      .filter((o) => !q || o.base.toLowerCase().includes(q) || o.buyExchange.includes(q) || o.sellExchange.includes(q))
      .sort((a, b) => b[sortKey] - a[sortKey])
  }, [opps, query, sortKey])

  const totalCount = opps.size
  const bestScore = rows[0]?.score ?? 0
  const bestSpread = rows[0]?.spreadPct ?? 0

  // Status labels
  const feedLabel =
    status === 'offline' ? 'Отключено'
    : status === 'connecting' ? 'Подключение…'
    : totalCount > 0 ? 'Данные поступают'
    : 'Ожидание первого скана'

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ST</div>
          <div>
            <strong>Spread Terminal</strong>
            <span>Профессиональный арбитраж</span>
          </div>
        </div>

        <nav>
          <button className={tab === 'opportunities' ? 'active' : ''} onClick={() => setTab('opportunities')}>
            Возможности
          </button>
          <button className={tab === 'coin' ? 'active' : ''} disabled={!selected} onClick={() => setTab('coin')}>
            {selected ? `Аналитика ${selected.base}` : 'Аналитика монеты'}
          </button>
          <button disabled>Сканер фандинга</button>
          <button disabled>Исполнение</button>
        </nav>

        <div className="connection-card">
          <span className={`status-dot ${status}`} />
          <div>
            <strong>{feedLabel}</strong>
            <span>Binance Spot + Perp</span>
          </div>
        </div>
      </aside>

      <main>
        <header className="topbar">
          <div>
            <span className="eyebrow">Межбиржевой арбитражный терминал</span>
            <h1>{tab === 'opportunities' ? 'Все возможности' : `${selected?.base ?? ''} — маршруты`}</h1>
          </div>
          <div className="live-pill">
            <span className={`status-dot ${status}`} />
            {totalCount} маршрутов
          </div>
        </header>

        {tab === 'opportunities' ? (
          <OpportunitiesTab
            rows={rows}
            query={query}
            setQuery={setQuery}
            sortKey={sortKey}
            setSortKey={setSortKey}
            bestScore={bestScore}
            bestSpread={bestSpread}
            totalCount={totalCount}
            status={status}
            onSelect={(o) => { setSelected(o); setTab('coin') }}
          />
        ) : (
          <CoinTab
            opp={selected}
            allOpps={Array.from(opps.values()).filter((o) => o.base === selected?.base)}
            onBack={() => setTab('opportunities')}
          />
        )}
      </main>
    </div>
  )
}

// ── Opportunities Tab ──────────────────────────────────────────────────────────
function OpportunitiesTab({
  rows, query, setQuery, sortKey, setSortKey,
  bestScore, bestSpread, totalCount, status, onSelect,
}: {
  rows: Opportunity[]
  query: string
  setQuery: (q: string) => void
  sortKey: SortKey
  setSortKey: (k: SortKey) => void
  bestScore: number
  bestSpread: number
  totalCount: number
  status: string
  onSelect: (o: Opportunity) => void
}) {
  return (
    <>
      <section className="metrics">
        <Metric label="Активных маршрутов" value={String(totalCount)} detail="По всем биржам и типам" />
        <Metric label="Лучший Score" value={bestScore.toFixed(1)} detail="Взвешенная оценка" />
        <Metric label="Лучший спред" value={`${bestSpread.toFixed(4)}%`} detail={rows[0] ? `${rows[0].buyExchange} → ${rows[0].sellExchange}` : '—'} />
        <Metric label="Статус" value={totalCount > 0 ? 'LIVE' : status.toUpperCase()} detail={API_WS} />
      </section>

      <section className="panel">
        <div className="filters">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Поиск BTC, ETH, SOL, binance…"
          />
          <select value={sortKey} onChange={(e) => setSortKey(e.target.value as SortKey)}>
            <option value="score">По Score</option>
            <option value="spreadPct">По спреду</option>
            <option value="volume24h">По объёму</option>
          </select>
        </div>

        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Монета</th>
                <th>Покупка</th>
                <th>Продажа</th>
                <th>Тип</th>
                <th>Спред</th>
                <th>Объём 24h</th>
                <th>OI</th>
                <th>Фандинг Δ</th>
                <th>Score</th>
              </tr>
            </thead>
            <tbody>
              {rows.slice(0, 300).map((row) => (
                <tr key={oppKey(row)} onClick={() => onSelect(row)}>
                  <td><strong>{row.base}</strong></td>
                  <td>
                    <span className="exchange-badge">{row.buyExchange}</span>
                    <span className="sub">{row.buySymbol}</span>
                  </td>
                  <td>
                    <span className="exchange-badge">{row.sellExchange}</span>
                    <span className="sub">{row.sellSymbol}</span>
                  </td>
                  <td><span className="route-badge">{routeLabel(row)}</span></td>
                  <td className={row.spreadPct > 0.5 ? 'positive' : 'neutral'}>
                    {row.spreadPct.toFixed(4)}%
                  </td>
                  <td>{fmtUSD(row.volume24h)}</td>
                  <td>{row.openInterest > 0 ? fmtUSD(row.openInterest) : '—'}</td>
                  <td className={row.fundingRate > 0 ? 'positive' : row.fundingRate < 0 ? 'negative' : 'neutral'}>
                    {row.fundingRate !== 0 ? `${(row.fundingRate * 100).toFixed(4)}%` : '—'}
                  </td>
                  <td><ScoreBadge score={row.score} /></td>
                </tr>
              ))}
            </tbody>
          </table>
          {rows.length === 0 && (
            <div className="empty">
              {status === 'live'
                ? 'Ожидание первого скана (~2с после старта)…'
                : 'Нет соединения с backend на порту 8080.'}
            </div>
          )}
        </div>
      </section>
    </>
  )
}

// ── Coin Tab ───────────────────────────────────────────────────────────────────
function CoinTab({
  opp, allOpps, onBack,
}: {
  opp: Opportunity | null
  allOpps: Opportunity[]
  onBack: () => void
}) {
  if (!opp) return <section className="panel empty">Выберите монету из таблицы.</section>

  const mid = (opp.buyAsk + opp.sellBid) / 2

  return (
    <>
      <button className="back" onClick={onBack}>← Назад к возможностям</button>
      <section className="coin-grid">
        <div className="panel chart-panel">
          <div className="panel-title">
            <div>
              <span className="eyebrow">Арбитражная пара</span>
              <h2>{opp.base}</h2>
            </div>
            <strong>${fmt(mid, 6)}</strong>
          </div>
          <SparkChart mid={mid} />
        </div>

        <div className="panel stats-panel">
          <h2>Параметры маршрута</h2>
          <Detail label="Покупка" value={`${opp.buyExchange} / ${opp.buySymbol} (${opp.buyType})`} />
          <Detail label="Продажа" value={`${opp.sellExchange} / ${opp.sellSymbol} (${opp.sellType})`} />
          <Detail label="Buy Ask" value={`$${fmt(opp.buyAsk, 8)}`} />
          <Detail label="Sell Bid" value={`$${fmt(opp.sellBid, 8)}`} />
          <Detail label="Спред" value={`${opp.spreadPct.toFixed(5)}%`} />
          <Detail label="Объём 24h" value={fmtUSD(opp.volume24h)} />
          <Detail label="OI" value={opp.openInterest > 0 ? fmtUSD(opp.openInterest) : '—'} />
          <Detail label="Фандинг Δ" value={opp.fundingRate !== 0 ? `${(opp.fundingRate * 100).toFixed(4)}%` : '—'} />
          <Detail label="Score" value={opp.score.toFixed(1)} />
        </div>
      </section>

      {allOpps.length > 1 && (
        <section className="panel">
          <h2 style={{ padding: '1rem 1.5rem 0.5rem', margin: 0, fontSize: '1rem' }}>
            Все маршруты для {opp.base} ({allOpps.length})
          </h2>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Покупка</th><th>Продажа</th><th>Тип</th>
                  <th>Спред</th><th>Фандинг Δ</th><th>Score</th>
                </tr>
              </thead>
              <tbody>
                {allOpps.sort((a, b) => b.score - a.score).map((r) => (
                  <tr key={oppKey(r)}>
                    <td>{r.buyExchange}<span className="sub">{r.buySymbol}</span></td>
                    <td>{r.sellExchange}<span className="sub">{r.sellSymbol}</span></td>
                    <td><span className="route-badge">{routeLabel(r)}</span></td>
                    <td className="positive">{r.spreadPct.toFixed(4)}%</td>
                    <td>{r.fundingRate !== 0 ? `${(r.fundingRate * 100).toFixed(4)}%` : '—'}</td>
                    <td><ScoreBadge score={r.score} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      <section className="analytics-grid">
        <AnalyticsCard title="Фандинг" value={opp.fundingRate !== 0 ? `${(opp.fundingRate * 100).toFixed(4)}%` : 'Нет данных'} detail="Разница ставок фандинга между биржами (из REST slow layer)." />
        <AnalyticsCard title="Открытый интерес" value={opp.openInterest > 0 ? fmtUSD(opp.openInterest) : 'Нет данных'} detail="Суммарный OI перп-контрактов по биржам." />
        <AnalyticsCard title="Basis (спот↔перп)" value={`${opp.spreadPct.toFixed(5)}%`} detail="Текущая премия/дисконт бессрочного контракта к споту." />
        <AnalyticsCard title="Score" value={opp.score.toFixed(1)} detail="Взвешенная оценка: спред 40%, объём 25%, OI 15%, фандинг 10%, ликвидность 10%." />
      </section>
    </>
  )
}

// ── Small components ───────────────────────────────────────────────────────────
function Metric({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{detail}</small>
    </div>
  )
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="detail">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function AnalyticsCard({ title, value, detail }: { title: string; value: string; detail: string }) {
  return (
    <div className="panel analytics-card">
      <span>{title}</span>
      <strong>{value}</strong>
      <p>{detail}</p>
    </div>
  )
}

function ScoreBadge({ score }: { score: number }) {
  const cls = score >= 70 ? 'score-high' : score >= 40 ? 'score-mid' : 'score-low'
  return <span className={`score-badge ${cls}`}>{score.toFixed(1)}</span>
}

function SparkChart({ mid }: { mid: number }) {
  const points = useMemo(
    () => Array.from({ length: 32 }, (_, i) => mid * (1 + Math.sin(i / 3) * 0.001 + i * 0.00002)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [Math.round(mid * 1000)] // recompute only on meaningful price change
  )
  const min = Math.min(...points)
  const max = Math.max(...points)
  const path = points
    .map((p, i) => `${(i / (points.length - 1)) * 100},${92 - ((p - min) / (max - min || 1)) * 80}`)
    .join(' ')
  return (
    <svg className="chart" viewBox="0 0 100 100" preserveAspectRatio="none">
      <polyline points={path} fill="none" stroke="currentColor" strokeWidth="2" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
