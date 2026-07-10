import { useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useConnection } from "./state/connection";
import { Sidebar } from "./components/shell/Sidebar";
import { TopBar } from "./components/shell/TopBar";
import { Connection } from "./panels/Connection";
import { Overview } from "./panels/Overview";
import { Providers } from "./panels/Providers";
import { Models } from "./panels/Models";
import { Usage } from "./panels/Usage";
import { Audit } from "./panels/Audit";
import { Projects } from "./panels/Projects";
import { Keys } from "./panels/Keys";
import { Policies } from "./panels/Policies";
import { Budgets } from "./panels/Budgets";
import { Tools } from "./panels/Tools";
import { Mcp } from "./panels/Mcp";
import { Playground } from "./panels/Playground";
import { Settings } from "./panels/Settings";

export function App() {
  const { status } = useConnection();
  const [navOpen, setNavOpen] = useState(false);

  if (status !== "connected") return <Connection />;

  return (
    <div className="app" data-nav-open={navOpen}>
      <Sidebar onNavigate={() => setNavOpen(false)} />
      {navOpen && (
        <div
          onClick={() => setNavOpen(false)}
          style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.4)", zIndex: 45 }}
        />
      )}
      <div className="main">
        <TopBar onMenu={() => setNavOpen((o) => !o)} />
        <div className="content">
          <Routes>
            <Route path="/overview" element={<Overview />} />
            <Route path="/usage" element={<Usage />} />
            <Route path="/providers" element={<Providers />} />
            <Route path="/models" element={<Models />} />
            <Route path="/audit" element={<Audit />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/keys" element={<Keys />} />
            <Route path="/policies" element={<Policies />} />
            <Route path="/budgets" element={<Budgets />} />
            <Route path="/tools" element={<Tools />} />
            <Route path="/mcp" element={<Mcp />} />
            <Route path="/playground" element={<Playground />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="*" element={<Navigate to="/overview" replace />} />
          </Routes>
        </div>
      </div>
    </div>
  );
}
