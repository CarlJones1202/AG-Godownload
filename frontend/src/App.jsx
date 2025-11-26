import { useState, useEffect } from 'react'
import { Routes, Route, NavLink, useLocation } from 'react-router-dom'
import './App.css'
import GalleryList from './components/GalleryList'
import SourceManager from './components/SourceManager'
import ImageList from './components/ImageList'

function App() {
    const location = useLocation()
    const [galleries, setGalleries] = useState([])
    const [sources, setSources] = useState([])
    const [images, setImages] = useState([])
    const [loading, setLoading] = useState(true)

    // Pagination state
    const [galleryPage, setGalleryPage] = useState(1)
    const [galleryMeta, setGalleryMeta] = useState({ total_pages: 1, current_page: 1 })
    const [sourcePage, setSourcePage] = useState(1)
    const [sourceMeta, setSourceMeta] = useState({ total_pages: 1, current_page: 1 })
    const [imagePage, setImagePage] = useState(1)
    const [imageMeta, setImageMeta] = useState({ total_pages: 1, current_page: 1 })

    useEffect(() => {
        if (location.pathname === '/' || location.pathname === '/galleries' || location.pathname.startsWith('/galleries/')) {
            fetchGalleries(1)
        } else if (location.pathname === '/sources') {
            fetchSources(1)
        } else if (location.pathname === '/images') {
            fetchImages(1)
        }
    }, [location.pathname])

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

    const fetchImages = async (page = 1) => {
        setLoading(true)
        try {
            const response = await fetch(`/api/images?page=${page}&limit=100`)
            const result = await response.json()
            if (result.data) {
                setImages(result.data)
                setImageMeta(result.meta)
                setImagePage(page)
            } else {
                setImages(result || [])
            }
        } catch (error) {
            console.error('Failed to fetch images:', error)
        } finally {
            setLoading(false)
        }
    }

    const handleSourceAdded = () => {
        fetchSources(1)
        fetchGalleries(1)
        fetchImages(1)
    }

    return (
        <div className="app">
            <header className="app-header">
                <h1>📸 Image Gallery</h1>
                <nav className="tabs">
                    <NavLink
                        to="/galleries"
                        className={({ isActive }) => isActive || location.pathname === '/' ? 'active' : ''}
                    >
                        Galleries
                    </NavLink>
                    <NavLink
                        to="/images"
                        className={({ isActive }) => isActive ? 'active' : ''}
                    >
                        Images
                    </NavLink>
                    <NavLink
                        to="/sources"
                        className={({ isActive }) => isActive ? 'active' : ''}
                    >
                        Sources
                    </NavLink>
                </nav>
            </header>

            <main className="app-main">
                {loading ? (
                    <div className="loading">Loading...</div>
                ) : (
                    <Routes>
                        <Route path="/" element={
                            <GalleryList
                                galleries={galleries}
                                onRefresh={() => fetchGalleries(galleryPage)}
                                meta={galleryMeta}
                                onPageChange={fetchGalleries}
                            />
                        } />
                        <Route path="/galleries" element={
                            <GalleryList
                                galleries={galleries}
                                onRefresh={() => fetchGalleries(galleryPage)}
                                meta={galleryMeta}
                                onPageChange={fetchGalleries}
                            />
                        } />
                        <Route path="/galleries/:id" element={
                            <GalleryList
                                galleries={galleries}
                                onRefresh={() => fetchGalleries(galleryPage)}
                                meta={galleryMeta}
                                onPageChange={fetchGalleries}
                            />
                        } />
                        <Route path="/images" element={
                            <ImageList
                                images={images}
                                onRefresh={() => fetchImages(imagePage)}
                                meta={imageMeta}
                                onPageChange={fetchImages}
                            />
                        } />
                        <Route path="/sources" element={
                            <SourceManager
                                sources={sources}
                                onSourceAdded={handleSourceAdded}
                                onRefresh={() => fetchSources(sourcePage)}
                                meta={sourceMeta}
                                onPageChange={fetchSources}
                            />
                        } />
                    </Routes>
                )}
            </main>
        </div>
    )
}

export default App
