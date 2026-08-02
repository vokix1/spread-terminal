# Spread Terminal

Real-time cryptocurrency spread scanner and market analytics terminal.

## v0.1 scope

- Binance spot instrument discovery through REST.
- Binance all-market best bid/ask stream through WebSocket.
- Internal concurrent event bus and browser WebSocket endpoint.
- Opportunities dashboard with search and sorting.
- Coin analytics view with live price, spread and top-of-book liquidity.
- Placeholders for spot/futures routes, funding, open interest, basis and depth validation.

## Run locally on Windows

### 1. Backend

Open PowerShell window 1:

```powershell
cd G:\projects\spread-terminal\backend
go mod tidy
go run ./cmd/server
```

Verify:

- `http://localhost:8080/health`
- `http://localhost:8080/api/instruments`

The backend log should show `Binance book ticker connected`.

### 2. Frontend

Open PowerShell window 2:

```powershell
cd G:\projects\spread-terminal\frontend
npm install
npm run dev
```

Open `http://localhost:5173`.

The first tab shows live Binance spot quotes. Click any row to open the second Coin Analytics tab.

## Docker

```powershell
docker compose up --build
```

Then open `http://localhost:5173`.

## Important limitations

This version is market-data-only. It does not place orders and must not be treated as an execution-ready arbitrage system. Current spreads are top-of-book venue spreads, not cross-exchange or VWAP-validated opportunities.

Next milestones:

1. OKX, Bybit and Hyperliquid adapters.
2. Spot-spot, spot-perpetual and perpetual-perpetual route construction.
3. Funding and open-interest feeds.
4. Local order books, VWAP and executable net edge.
5. Historical storage, replay and paper execution.
