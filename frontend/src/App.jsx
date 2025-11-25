import { useState, useEffect } from 'react'
import './App.css'
import GalleryList from './components/GalleryList'
import SourceManager from './components/SourceManager'

function App() {
    const [activeTab, setActiveTab] = useState('galleries')
    const [galleries, setGalleries] = useState([])
    const [sources, setSources] = useState([])
    const [loading, setLoading] = useState(true)

    useEffect(() => {
        fetchGalleries()
        fetchSources()
    }, [])

    const fetchGalleries = async () => {
        try {
            const response = await fetch('/api/galleries')
            const data = await response.json()
            setGalleries(data || [])
        } catch (error) {
            console.error('Failed to fetch galleries:', error)
        } finally {
            setLoading(false)
        }
    }

    const fetchSources = async () => {
        try {
            const response = await fetch('/api/sources')
            const data = await response.json()
            setSources(data || [])
        } catch (error) {
            console.error('Failed to fetch sources:', error)
        }
    }

    const handleSourceAdded = () => {
        fetchSources()
        fetchGalleries()
    }

    return (
        <div className="app">
            <header className="app-header">
                <h1>📸 Image Gallery</h1>
                <nav className="tabs">
                    <button
                        className={activeTab === 'galleries' ? 'active' : ''}
                        onClick={() => setActiveTab('galleries')}
                    >
                        Galleries
                    </button>
                    <button
                        className={activeTab === 'sources' ? 'active' : ''}
                        onClick={() => setActiveTab('sources')}
                    >
                        Sources
                    </button>
                </nav>
            </header>

            <main className="app-main">
                {loading ? (
                    <div className="loading">Loading...</div>
                ) : (
                    <>
                        {activeTab === 'galleries' && (
                            <GalleryList galleries={galleries} onRefresh={fetchGalleries} />
                        )}
                        {activeTab === 'sources' && (
                            <SourceManager
                                sources={sources}
                                onSourceAdded={handleSourceAdded}
                                onRefresh={fetchSources}
                            />
                        )}
                    </>
                )}
            </main>
        </div>
    )
}

export default App
