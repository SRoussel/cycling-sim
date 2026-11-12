import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Home from './pages/Home'
import RaceSetup from './pages/RaceSetup'
import Simulation from './pages/Simulation'
import Results from './pages/Results'
import StageRaceSetup from './pages/StageRaceSetup'
import StageRaceGC from './pages/StageRaceGC'
import StageRaceStages from './pages/StageRaceStages'
import StageRaceSimulation from './pages/StageRaceSimulation'
import Validation from './pages/Validation'
import Settings from './pages/Settings'
import Seasons from './pages/Seasons'
import SeasonHub from './pages/SeasonHub'

function App() {
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<Home />} />
        <Route path="race-setup/:raceId" element={<RaceSetup />} />
        <Route path="simulation/:raceId" element={<Simulation />} />
        <Route path="results/:raceId" element={<Results />} />
        <Route path="stage-race/:stageRaceId" element={<StageRaceSetup />} />
        <Route path="stage-race-sim/:stageRaceId" element={<StageRaceSimulation />} />
        <Route path="stage-race-gc/:stageRaceId" element={<StageRaceGC />} />
        <Route path="stage-race-stages/:stageRaceId" element={<StageRaceStages />} />
        <Route path="season" element={<Seasons />} />
        <Route path="season/:seasonId" element={<SeasonHub />} />
        <Route path="validation" element={<Validation />} />
        <Route path="settings" element={<Settings />} />
      </Route>
    </Routes>
  )
}

export default App
