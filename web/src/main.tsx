import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

type Event = { id: string; status: string; location: string; note: string; at: string };
type Shipment = { id: string; recipient: string; destination: string; eta: string; events: Event[] };
type Health = { service: string; status: string; dependencies: { name: string; status: string; latencyMs: number }[] };

const api = import.meta.env.VITE_API_URL ?? "http://localhost:8080/api";

function App() {
  const [shipments, setShipments] = useState<Shipment[]>([]);
  const [health, setHealth] = useState<Health | null>(null);
  const [selectedID, setSelectedID] = useState("PP-1042");
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([
      fetch(`${api}/shipments`).then((r) => r.ok ? r.json() : Promise.reject(r.statusText)),
      fetch(`${api}/health`).then((r) => r.ok ? r.json() : Promise.reject(r.statusText))
    ]).then(([shipmentPayload, healthPayload]) => {
      setShipments(shipmentPayload.shipments);
      setHealth(healthPayload);
    }).catch(() => setError("The local services are unavailable. Start Docker Compose, then refresh."));
  }, []);

  const selected = useMemo(() => shipments.find((shipment) => shipment.id === selectedID) ?? shipments[0], [shipments, selectedID]);
  const status = selected?.events.at(-1)?.status.replaceAll("_", " ") ?? "loading";

  return <main>
    <header>
      <div>
        <p className="eyebrow">OPERATIONS CONSOLE</p>
        <h1>ParcelPulse</h1>
      </div>
      <div className={health?.status === "healthy" ? "health good" : "health"}>
        <span className="dot" /> {health?.status ?? "checking services"}
      </div>
    </header>

    {error && <div className="notice">{error}</div>}

    <section className="metrics" aria-label="Shipment summary">
      <Metric label="Active shipments" value={shipments.length || "—"} detail="in current demo queue" />
      <Metric label="Needs attention" value={shipments.filter((s) => s.events.at(-1)?.status === "exception").length} detail="exception events" />
      <Metric label="Services" value={health?.dependencies.filter((d) => d.status === "healthy").length ?? "—"} detail="healthy dependencies" />
    </section>

    <section className="workspace">
      <aside>
        <div className="section-title"><h2>Shipments</h2><span>{shipments.length}</span></div>
        {shipments.map((shipment) => <button className={shipment.id === selected?.id ? "shipment active" : "shipment"} key={shipment.id} onClick={() => setSelectedID(shipment.id)}>
          <strong>{shipment.id}</strong><span>{shipment.destination}</span><em>{shipment.events.at(-1)?.status.replaceAll("_", " ")}</em>
        </button>)}
      </aside>

      <article className="timeline">
        {selected ? <>
          <div className="shipment-head">
            <div><p className="eyebrow">TRACKING {selected.id}</p><h2>{selected.recipient}</h2><p>{selected.destination} · ETA {new Date(selected.eta).toLocaleString()}</p></div>
            <span className="pill">{status}</span>
          </div>
          <ol>
            {[...selected.events].reverse().map((event) => <li key={event.id}>
              <span className="event-dot" /><div><h3>{event.status.replaceAll("_", " ")}</h3><p>{event.location} · {event.note}</p><time>{new Date(event.at).toLocaleString()}</time></div>
            </li>)}
          </ol>
        </> : <p>Loading shipments…</p>}
      </article>
    </section>
  </main>;
}

function Metric({ label, value, detail }: { label: string; value: string | number; detail: string }) {
  return <article className="metric"><p>{label}</p><strong>{value}</strong><span>{detail}</span></article>;
}

createRoot(document.getElementById("root")!).render(<App />);
