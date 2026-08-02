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
  return new Intl.NumberFormat('ru-RU', { maximumFractionDigits: digits }).format(value)
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
        if (!event.symbol || !event.bid || !event.ask) return
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
  const quoteCount = Object.keys(quotes).length
  const feedLabel = status === 'offline'
    ? 'Отключено'
    : status === 'connecting'
      ? 'Подключение'
      : quoteCount > 0
        ? 'Котировки поступают'
        : 'Канал подключён, данных пока нет'

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ST</div>
          <div><strong>Spread Terminal</strong><span>Аналитика крипторынка</span></div>
        </div>
        <nav>
          <button className={tab === 'opportunities' ? 'active' : ''} onClick={() => setTab('opportunities')}>Возможности</button>
          <button className={tab === 'coin' ? 'active' : ''} onClick={() => setTab('coin')}>Аналитика монеты</button>
          <button disabled>Сканер фандинга</button>
          <button disabled>Исполнение</button>
          <button disabled>Система</button>
        </nav>
        <div className="connection-card">
          <span className={`status-dot ${status}`} />
          <div><strong>{feedLabel}</strong><span>Binance: лучшие bid/ask</span></div>
        </div>
      </aside>

      <main>
        <header className="topbar">
          <div><span className="eyebrow">Рабочее пространство криптоарбитража</span><h1>{tab === 'opportunities' ? 'Все возможности' : `Аналитика ${selected.replace('USDT', '')}`}</h1></div>
          <div className="live-pill"><span className={`status-dot ${status}`} />{quoteCount} котировок</div>
        </header>

        {tab === 'opportunities' ? (
          <>
            <section className="metrics">
              <Metric label="Отслеживаемые рынки" value={String(quoteCount)} detail="Лучшие цены в реальном времени" />
              <Metric label="Видимые пары USDT" value={String(rows.length)} detail="Отфильтрованный список" />
              <Metric label="Лучший спред" value={rows[0] ? `${rows[0].spread.toFixed(4)}%` : '—'} detail={rows[0]?.symbol ?? 'Ожидание данных'} />
              <Metric label="Статус потока" value={quoteCount > 0 ? 'ДАННЫЕ ИДУТ' : status === 'live' ? 'ОЖИДАНИЕ' : status.toUpperCase()} detail={API_WS} />
            </section>

            <section className="panel">
              <div className="filters">
                <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск BTC, ETH, SOL…" />
                <select value={routeType} onChange={(event) => setRouteType(event.target.value as RouteType)}>
                  <option value="all">Все маршруты</option>
                  <option value="spot-spot">Спот ↔ Спот</option>
                  <option value="spot-future" disabled>Спот ↔ Фьючерс</option>
                  <option value="future-future" disabled>Фьючерс ↔ Фьючерс</option>
                </select>
                <select value={sortKey} onChange={(event) => setSortKey(event.target.value as typeof sortKey)}>
                  <option value="spread">Сортировать по спреду</option>
                  <option value="liquidity">Сортировать по ликвидности</option>
                  <option value="symbol">Сортировать по символу</option>
                </select>
              </div>

              <div className="table-wrap">
                <table>
                  <thead><tr><th>Монета</th><th>Маршрут</th><th>Bid</th><th>Ask</th><th>Спред</th><th>Ликвидность на лучших ценах</th><th>Обновлено</th></tr></thead>
                  <tbody>
                    {rows.slice(0, 200).map((row) => (
                      <tr key={row.symbol} onClick={() => { setSelected(row.symbol); setTab('coin') }}>
                        <td><strong>{row.symbol.replace('USDT', '')}</strong><span className="sub">{row.symbol}</span></td>
                        <td><span className="route-badge">Binance спот</span></td>
                        <td>{formatNumber(row.bid, 8)}</td>
                        <td>{formatNumber(row.ask, 8)}</td>
                        <td className="positive">{row.spread.toFixed(5)}%</td>
                        <td>${formatNumber(row.liquidity, 0)}</td>
                        <td>{row.time ? new Date(row.time).toLocaleTimeString('ru-RU') : 'сейчас'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {rows.length === 0 && <div className="empty">{status === 'live' ? 'Соединение с backend установлено, но котировки Binance пока не поступили. Перезапустите backend после обновления ветки.' : 'Нет соединения с backend на порту 8080.'}</div>}
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
  if (!quote) return <section className="panel empty">Котировка ещё не выбрана.</section>
  const spread = quote.ask > 0 ? ((quote.ask - quote.bid) / quote.ask) * 100 : 0
  const mid = (quote.bid + quote.ask) / 2
  const points = Array.from({ length: 28 }, (_, index) => mid * (1 + Math.sin(index / 3) * 0.0008 + index * 0.00001))
  const min = Math.min(...points)
  const max = Math.max(...points)
  const path = points.map((point, index) => `${(index / (points.length - 1)) * 100},${92 - ((point - min) / (max - min || 1)) * 80}`).join(' ')

  return <>
    <button className="back" onClick={onBack}>← Назад к возможностям</button>
    <section className="coin-grid">
      <div className="panel chart-panel">
        <div className="panel-title"><div><span className="eyebrow">Обзор цены</span><h2>{quote.symbol}</h2></div><strong>${formatNumber(mid, 8)}</strong></div>
        <svg className="chart" viewBox="0 0 100 100" preserveAspectRatio="none"><polyline points={path} fill="none" stroke="currentColor" strokeWidth="2" vectorEffect="non-scaling-stroke" /></svg>
      </div>
      <div className="panel stats-panel">
        <h2>Параметры рынка</h2>
        <Detail label="Биржа" value={quote.exchange} />
        <Detail label="Лучший bid" value={formatNumber(quote.bid, 8)} />
        <Detail label="Лучший ask" value={formatNumber(quote.ask, 8)} />
        <Detail label="Спред" value={`${spread.toFixed(5)}%`} />
        <Detail label="Объём bid" value={formatNumber(quote.bidQty, 6)} />
        <Detail label="Объём ask" value={formatNumber(quote.askQty, 6)} />
      </div>
    </section>
    <section className="analytics-grid">
      <AnalyticsCard title="Фандинг" value="Ожидает фьючерсный адаптер" detail="Сравнение ставок Binance, OKX, Bybit и Hyperliquid появится на следующем этапе." />
      <AnalyticsCard title="Открытый интерес" value="Ожидает поток OI" detail="Текущий OI, изменения за 1 час и 24 часа, распределение по биржам." />
      <AnalyticsCard title="Базис" value="Ожидает маршруты спот-перпетуал" detail="Премия бессрочного контракта и годовая доходность carry-сделки." />
      <AnalyticsCard title="Исполнимый спред" value={`${spread.toFixed(5)}%`} detail="Пока используется только лучшая цена. Проверка глубины и VWAP будет добавлена далее." />
    </section>
  </>
}

function Detail({ label, value }: { label: string; value: string }) { return <div className="detail"><span>{label}</span><strong>{value}</strong></div> }
function AnalyticsCard({ title, value, detail }: { title: string; value: string; detail: string }) { return <div className="panel analytics-card"><span>{title}</span><strong>{value}</strong><p>{detail}</p></div> }

export default App
