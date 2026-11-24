import { Outlet, NavLink } from 'react-router-dom'
import './Layout.css'
import { RiderProfileHost } from './RiderProfile'

function Layout() {
  return (
    <div className="layout">
      <nav className="navbar">
        <div className="nav-brand">
          <svg className="brand-icon" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <circle cx="5.5" cy="17" r="3.5" /><circle cx="18.5" cy="17" r="3.5" /><path d="M5.5 17 9 9h6l3.5 8M9 9 7.5 6H5M15 9l-2-4h3" />
          </svg>
          <span className="brand-text">Cycling Sim</span>
        </div>
        <ul className="nav-links">
          <li><NavLink to="/" end>Races</NavLink></li>
          <li><NavLink to="/season">Season</NavLink></li>
          <li><NavLink to="/validation">Validation</NavLink></li>
          <li><NavLink to="/settings">Settings</NavLink></li>
        </ul>
      </nav>
      <main className="main-content">
        <Outlet />
      </main>
      <RiderProfileHost />
    </div>
  )
}

export default Layout
