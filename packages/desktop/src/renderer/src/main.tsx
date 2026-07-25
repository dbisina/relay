import React from 'react'
import ReactDOM from 'react-dom/client'
import './lib/ensureBridge'
import { App } from './App'
import { loadSavedAccent } from './lib/theme'
import './styles/tokens.css'
import './styles/global.css'

loadSavedAccent()

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
