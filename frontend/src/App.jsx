import { useState, useEffect } from 'react'
import './App.css'
import GalleryList from './components/GalleryList'
import SourceManager from './components/SourceManager'

function App() {
    const [activeTab, setActiveTab] = useState('galleries')
    const [galleries, setGalleries] = useState([])
    const [sources, setSources] = useState([])
    const [loading, setLoading] = useState(true)

    // Pagination state
    const [galleryPage, setGalleryPage] = useState(1)
    const [galleryMeta, setGalleryMeta] = useState({ total_pages: 1, current_page: 1 })
    const [sourcePage, setSourcePage] = useState(1)
    const [sourceMeta, setSourceMeta] = useState({ total_pages: 1, current_page: 1 })

    useEffect(() => {
        fetchGalleries(1)
        fetchSources(1)
    }, [])

    const fetchGalleries = async (page = 1) => {
        setLoading(true)
        try {
            const response = await fetch(`/api/galleries?page=${page}&limit=50`)
            const result = await response.json()
            if (result.data) {
                setGalleries(result.data)
                setGalleryMeta(result.meta)
                setGalleryPage(page)
            } else {
                setGalleries(result || [])
            }
        } catch (error) {
            console.error('Failed to fetch galleries:', error)
        } finally {
            setLoading(false)
        }
    }

    const fetchSources = async (page = 1) => {
        try {
            const response = await fetch(`/api/sources?page=${page}&limit=50`)
            const result = await response.json()
            if (result.data) {
                setSources(result.data)
                setSourceMeta(result.meta)
                setSourcePage(page)
            } else {
                setSources(result || [])
            }
        } catch (error) {
            console.error('Failed to fetch sources:', error)
        }
    }

    const handleSourceAdded = () => {
        fetchSources(1)
        fetchGalleries(1)
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
                            <GalleryList
                                galleries={galleries}
                                onRefresh={() => fetchGalleries(galleryPage)}
                                meta={galleryMeta}
                                onPageChange={fetchGalleries}
                            />
                        )}
                        {activeTab === 'sources' && (
                            <SourceManager
                                sources={sources}
                                onSourceAdded={handleSourceAdded}
                                onRefresh={() => fetchSources(sourcePage)}
                                meta={sourceMeta}
                                onPageChange={fetchSources}
                            />
                        )}
                    </>
                )}
            </main>
        </div>
    )
}

export default App
