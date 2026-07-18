import { useState } from 'react'

function App() {
  const [count, setCount] = useState(0)

  return (
    <div className="min-h-screen bg-gray-50 p-8">
      <h1 className="text-3xl font-bold text-gray-900">DBHUB</h1>
      <p className="mt-4 text-gray-600">集中式数据库查询与导出平台</p>
      <button
        type="button"
        onClick={() => setCount((c) => c + 1)}
        className="mt-4 rounded bg-blue-600 px-4 py-2 text-white hover:bg-blue-700"
      >
        count is {count}
      </button>
    </div>
  )
}

export default App
