import { useState, useEffect } from 'react'
import { Routes, Route, NavLink, useLocation, useSearchParams } from 'react-router-dom'
import './App.css'
import GalleryList from './components/GalleryList'
import SourceManager from './components/SourceManager'
import ImageList from './components/ImageList'
import PersonList from './components/PersonList'
import PersonDetail from './components/PersonDetail'
import SearchBar from './components/SearchBar'

function App() {
    const location = useLocation()
    const [searchParams, setSearchParams] = useSearchParams()
    const [galleries, setGalleries] = useState([])
    const [sources, setSources] = useState([])
    const [images, setImages] = useState([])
    const [people, setPeople] = useState([])
    const [loading, setLoading] = useState(true)

    // Pagination state
    const [galleryPage, setGalleryPage] = useState(1)
    const [galleryMeta, setGalleryMeta] = useState({ total_pages: 1, current_page: 1 })
    const [sourcePage, setSourcePage] = useState(1)
    const [sourceMeta, setSourceMeta] = useState({ total_pages: 1, current_page: 1 })
    const [imagePage, setImagePage] = useState(1)
    const [imageMeta, setImageMeta] = useState({ total_pages: 1, current_page: 1 })
    const [personPage, setPersonPage] = useState(1)
    const [personMeta, setPersonMeta] = useState({ total_pages: 1, current_page: 1 })

    useEffect(() => {
        // Get page from URL query params
        const pageFromUrl = parseInt(searchParams.get('page')) || 1

        if (location.pathname === '/' || location.pathname === '/galleries' || location.pathname.startsWith('/galleries/')) {
            fetchGalleries(pageFromUrl)
        } else if (location.pathname === '/sources') {
            fetchSources(pageFromUrl)
        } else if (location.pathname === '/images') {
            fetchImages(pageFromUrl)
        } else if (location.pathname === '/people') {
            fetchPeople(pageFromUrl)
        } else {
            setLoading(false)
        }
    }, [location.pathname, searchParams])

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

    const fetchSources = async (page = 1, search = '') => {
        // Only set global loading if we don't have data yet (initial load)
        if (sources.length === 0) {
            setLoading(true)
        }
        try {
            const query = search ? `&q=${encodeURIComponent(search)}` : ''
            const response = await fetch(`/api/sources?page=${page}&limit=50${query}`)
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
        } finally {
            setLoading(false)
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

    const fetchPeople = async (page = 1) => {
        setLoading(true)
        try {
            const response = await fetch(`/api/people?page=${page}&limit=50`)
            const result = await response.json()
            if (result.data) {
                setPeople(result.data)
                setPersonMeta(result.meta)
                setPersonPage(page)
            } else {
                setPeople(result || [])
            }
        } catch (error) {
            console.error('Failed to fetch people:', error)
        } finally {
            setLoading(false)
        }
    }

    const handleSourceAdded = () => {
        fetchSources(1)
        fetchGalleries(1)
        fetchImages(1)
    }

    const handleGalleryPageChange = (page) => {
        setSearchParams({ page: page.toString() })
        fetchGalleries(page)
    }

    const handleSourcePageChange = (page) => {
        setSearchParams({ page: page.toString() })
        fetchSources(page)
    }

    const handleImagePageChange = (page) => {
        setSearchParams({ page: page.toString() })
        fetchImages(page)
    }

    const handlePersonPageChange = (page) => {
        setSearchParams({ page: page.toString() })
        fetchPeople(page)
    }

    return (
        <div className="app">
            <header className="app-header">
                <div className="header-top">
                    <h1>📸 Image Gallery</h1>
                    <SearchBar />
                </div>
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
                    <NavLink
                        to="/people"
                        className={({ isActive }) => isActive ? 'active' : ''}
                    >
                        People
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
                                onPageChange={handleGalleryPageChange}
                            />
                        } />
                        <Route path="/galleries" element={
                            <GalleryList
                                galleries={galleries}
                                onRefresh={() => fetchGalleries(galleryPage)}
                                meta={galleryMeta}
                                onPageChange={handleGalleryPageChange}
                            />
                        } />
                        <Route path="/galleries/:id" element={
                            <GalleryList
                                galleries={galleries}
                                onRefresh={() => fetchGalleries(galleryPage)}
                                meta={galleryMeta}
                                onPageChange={handleGalleryPageChange}
                            />
                        } />
                        <Route path="/images" element={
                            <ImageList
                                images={images}
                                onRefresh={() => fetchImages(imagePage)}
                                meta={imageMeta}
                                onPageChange={handleImagePageChange}
                            />
                        } />
                        <Route path="/sources" element={
                            <SourceManager
                                sources={sources}
                                onSourceAdded={handleSourceAdded}
                                onRefresh={() => fetchSources(sourcePage)}
                                meta={sourceMeta}
                                onPageChange={handleSourcePageChange}
                                onSearch={(query) => fetchSources(1, query)}
                            />
                        } />
                        <Route path="/people" element={
                            <PersonList
                                people={people}
                                onRefresh={() => fetchPeople(personPage)}
                                meta={personMeta}
                                onPageChange={handlePersonPageChange}
                            />
                        } />
                        <Route path="/people/:id" element={<PersonDetail />} />
                    </Routes>
                )}
            </main>
        </div>
    )
}

export default App
