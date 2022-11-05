import * as React from 'react'
import {createRoot} from 'react-dom/client'

function App() {
  return (
    <div className="App">
      <h1>Hello World</h1>
    </div>
  );
}

const container = document.getElementById('root')
const root = createRoot(container)
root.render(<App />)

fetch("/traces").then(response => response.json()).then(data => console.log(data))