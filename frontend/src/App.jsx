import { useState, useEffect, useCallback } from 'react'
import { Routes, Route, NavLink, useLocation, useSearchParams, useNavigate } from 'react-router-dom'
import './App.css'
import GalleryList from './components/GalleryList'
import SourceManager from './components/SourceManager'
import ImageList from './components/ImageList'
import VideoList from './components/VideoList'
import PersonList from './components/PersonList'
import PersonDetail from './components/PersonDetail'
import SearchBar from './components/SearchBar'
import GalleryDetail from './components/GalleryDetail'

function App() {
    const location = useLocation()
    const navigate = useNavigate()
    const [searchParams, setSearchParams] = useSearchParams()
    const [galleries, setGalleries] = useState([])
    const [sources, setSources] = useState([])
    const [images, setImages] = useState([])
    const [videos, setVideos] = useState([])
    const [people, setPeople] = useState([])
    const [favorites, setFavorites] = useState([])
    const [loading, setLoading] = useState(true)
    const [colorSearchActive, setColorSearchActive] = useState(false)
    const [searchColor, setSearchColor] = useState(null)

    // Pagination state
    const [galleryPage, setGalleryPage] = useState(1)
    const [galleryMeta, setGalleryMeta] = useState({ total_pages: 1, current_page: 1 })
    const [sourcePage, setSourcePage] = useState(1)
    const [sourceMeta, setSourceMeta] = useState({ total_pages: 1, current_page: 1 })
    const [imagePage, setImagePage] = useState(1)
    const [imageMeta, setImageMeta] = useState({ total_pages: 1, current_page: 1 })
    const [favoritePage, setFavoritePage] = useState(1)
    const [favoriteMeta, setFavoriteMeta] = useState({ total_pages: 1, current_page: 1 })
    const [videoPage, setVideoPage] = useState(1)
    const [videoMeta, setVideoMeta] = useState({ total_pages: 1, current_page: 1 })
    const [personPage, setPersonPage] = useState(1)
    const [personMeta, setPersonMeta] = useState({ total_pages: 1, current_page: 1 })


    const fetchGalleries = useCallback(async (page = 1, sort = 'newest', seed = '0') => {
        setLoading(true)
        try {
            const response = await fetch(`/api/galleries?page=${page}&limit=50&sort=${sort}&seed=${seed}`)
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
    }, [])

    const fetchSources = useCallback(async (page = 1, search = '') => {
        try {
            const query = search ? `&q=${encodeURIComponent(search)}` : ''
            const response = await fetch(`/api/sources?page=${page}&limit=12${query}`)
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
    }, [])

    const fetchImages = useCallback(async (page = 1, sort = 'newest', seed = '0') => {
        setLoading(true)
        try {
            const response = await fetch(`/api/images?page=${page}&limit=100&sort=${sort}&seed=${seed}`)
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
    }, [])

    const fetchFavorites = useCallback(async (page = 1, sort = 'newest', seed = '0') => {
        setLoading(true)
        try {
            const response = await fetch(`/api/images?favorites=true&page=${page}&limit=100&sort=${sort}&seed=${seed}`)
            const result = await response.json()
            if (result.data) {
                setFavorites(result.data)
                setFavoriteMeta(result.meta)
                setFavoritePage(page)
            } else {
                setFavorites(result || [])
            }
        } catch (error) {
            console.error('Failed to fetch favorites:', error)
        } finally {
            setLoading(false)
        }
    }, [])

    const fetchPeople = useCallback(async (page = 1) => {
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
    }, [])

    const fetchVideos = useCallback(async (page = 1, sort = 'newest', seed = '0') => {
        setLoading(true)
        try {
            const response = await fetch(`/api/images?type=video&page=${page}&limit=50&sort=${sort}&seed=${seed}`)
            const result = await response.json()
            if (result.data) {
                setVideos(result.data)
                setVideoMeta(result.meta)
                setVideoPage(page)
            } else {
                setVideos(result || [])
            }
        } catch (error) {
            console.error('Failed to fetch videos:', error)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => {
        // Get params from URL
        const pageFromUrl = parseInt(searchParams.get('page')) || 1
        const sortFromUrl = searchParams.get('sort') || 'newest'
        const seedFromUrl = searchParams.get('seed') || '0'

        if (location.pathname === '/' || location.pathname === '/galleries' || location.pathname.startsWith('/galleries/')) {
            fetchGalleries(pageFromUrl, sortFromUrl, seedFromUrl)
        } else if (location.pathname === '/sources') {
            const searchQ = searchParams.get('q') || ''
            fetchSources(pageFromUrl, searchQ)
        } else if (location.pathname === '/images') {
            fetchImages(pageFromUrl, sortFromUrl, seedFromUrl)
        } else if (location.pathname === '/videos') {
            fetchVideos(pageFromUrl, sortFromUrl, seedFromUrl)
        } else if (location.pathname === '/favorites') {
            fetchFavorites(pageFromUrl, sortFromUrl, seedFromUrl)
        } else if (location.pathname === '/people') {
            fetchPeople(pageFromUrl)
        } else {
            setLoading(false)
        }
    }, [location.pathname, searchParams, fetchGalleries, fetchSources, fetchImages, fetchVideos, fetchFavorites, fetchPeople])

    const handleRefreshGalleries = useCallback(() => {
        fetchGalleries(galleryPage, searchParams.get('sort') || 'newest', searchParams.get('seed') || '0')
    }, [fetchGalleries, galleryPage, searchParams])

    const handleRefreshSources = useCallback(() => {
        fetchSources(sourcePage, searchParams.get('q') || '')
    }, [fetchSources, sourcePage, searchParams])

    const handleRefreshImages = useCallback(() => {
        fetchImages(imagePage, searchParams.get('sort') || 'newest', searchParams.get('seed') || '0')
    }, [fetchImages, imagePage, searchParams])

    const handleRefreshVideos = useCallback(() => {
        fetchVideos(videoPage, searchParams.get('sort') || 'newest', searchParams.get('seed') || '0')
    }, [fetchVideos, videoPage, searchParams])

    const handleRefreshFavorites = useCallback(() => {
        fetchFavorites(favoritePage, searchParams.get('sort') || 'newest', searchParams.get('seed') || '0')
    }, [fetchFavorites, favoritePage, searchParams])

    const handleRefreshPeople = useCallback(() => {
        fetchPeople(personPage)
    }, [fetchPeople, personPage])

    const handleSourceAdded = () => {
        // Clear search to show the new source
        setSearchParams(prev => {
            prev.delete('q')
            prev.set('page', '1')
            return prev
        })
        fetchSources(1)
        fetchGalleries(1)
        fetchImages(1)
    }

    const handleSortChange = (newSort) => {
        setSearchParams(prev => {
            prev.set('sort', newSort)
            if (newSort !== 'shuffle') {
                prev.delete('seed')
            } else if (!prev.get('seed')) {
                prev.set('seed', Math.floor(Math.random() * 1000000).toString())
            }
            prev.set('page', '1')
            return prev
        })
    }

    const handleSeedChange = (newSeed) => {
        setSearchParams(prev => {
            prev.set('seed', newSeed.toString())
            prev.set('page', '1')
            return prev
        })
    }

    const handleGalleryPageChange = (page) => {
        setSearchParams(prev => {
            prev.set('page', page.toString())
            return prev
        })
    }

    const handleSourcePageChange = (page) => {
        setSearchParams(prev => {
            prev.set('page', page.toString())
            return prev
        })
    }

    const handleImagePageChange = (page) => {
        setSearchParams(prev => {
            prev.set('page', page.toString())
            return prev
        })
    }

    const handleVideoPageChange = (page) => {
        setSearchParams(prev => {
            prev.set('page', page.toString())
            return prev
        })
    }

    const handleFavoritePageChange = (page) => {
        setSearchParams(prev => {
            prev.set('page', page.toString())
            return prev
        })
    }

    const handlePersonPageChange = (page) => {
        setSearchParams(prev => {
            prev.set('page', page.toString())
            return prev
        })
    }

    const searchByColor = async (color) => {
        setLoading(true)
        setColorSearchActive(true)
        setSearchColor(color)
        try {
            const response = await fetch(`/api/search/color?color=${encodeURIComponent(color)}&threshold=30&limit=100`)
            const result = await response.json()
            if (result.data) {
                setImages(result.data)
                setImageMeta(result.meta)
                setImagePage(1)
            } else {
                setImages([])
            }
        } catch (error) {
            console.error('Failed to search by color:', error)
        } finally {
            setLoading(false)
        }
    }

    const clearColorSearch = () => {
        setColorSearchActive(false)
        setSearchColor(null)
        fetchImages(1)
    }

    return (
        <div className="app">
            <header className="app-header">
                <div className="header-top">
                    <h1>📸 Image Gallery</h1>
                    <SearchBar
                        onColorFilter={searchByColor}
                        onPersonFilter={(person) => navigate(`/people/${person.id}`)}
                    />
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
                        to="/videos"
                        className={({ isActive }) => isActive ? 'active' : ''}
                    >
                        Videos
                    </NavLink>
                    <NavLink
                        to="/favorites"
                        className={({ isActive }) => isActive ? 'active' : ''}
                    >
                        Favorites
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
                                onRefresh={handleRefreshGalleries}
                                meta={galleryMeta}
                                onPageChange={handleGalleryPageChange}
                                sort={searchParams.get('sort') || 'newest'}
                                setSort={handleSortChange}
                                seed={parseInt(searchParams.get('seed')) || 0}
                                setSeed={handleSeedChange}
                            />
                        } />
                        <Route path="/galleries" element={
                            <GalleryList
                                galleries={galleries}
                                onRefresh={handleRefreshGalleries}
                                meta={galleryMeta}
                                onPageChange={handleGalleryPageChange}
                                sort={searchParams.get('sort') || 'newest'}
                                setSort={handleSortChange}
                                seed={parseInt(searchParams.get('seed')) || 0}
                                setSeed={handleSeedChange}
                            />
                        } />
                        <Route path="/galleries/:id" element={<GalleryDetail />} />
                        <Route path="/images" element={
                            <ImageList
                                images={images}
                                onRefresh={handleRefreshImages}
                                meta={imageMeta}
                                onPageChange={handleImagePageChange}
                                sort={searchParams.get('sort') || 'newest'}
                                setSort={handleSortChange}
                                seed={parseInt(searchParams.get('seed')) || 0}
                                setSeed={handleSeedChange}
                            />
                        } />
                        <Route path="/videos" element={
                            <VideoList
                                videos={videos}
                                onRefresh={handleRefreshVideos}
                                meta={videoMeta}
                                onPageChange={handleVideoPageChange}
                                sort={searchParams.get('sort') || 'newest'}
                                setSort={handleSortChange}
                                seed={parseInt(searchParams.get('seed')) || 0}
                                setSeed={handleSeedChange}
                            />
                        } />
                        <Route path="/favorites" element={
                            <ImageList
                                images={favorites}
                                onRefresh={handleRefreshFavorites}
                                meta={favoriteMeta}
                                onPageChange={handleFavoritePageChange}
                                sort={searchParams.get('sort') || 'newest'}
                                setSort={handleSortChange}
                                seed={parseInt(searchParams.get('seed')) || 0}
                                setSeed={handleSeedChange}
                            />
                        } />
                        <Route path="/sources" element={
                            <SourceManager
                                sources={sources}
                                onSourceAdded={handleSourceAdded}
                                onRefresh={handleRefreshSources}
                                meta={sourceMeta}
                                onPageChange={handleSourcePageChange}
                                onSearch={(query) => {
                                    setSearchParams(prev => {
                                        if (query) prev.set('q', query)
                                        else prev.delete('q')
                                        prev.set('page', '1')
                                        return prev
                                    })
                                }}
                                searchQuery={searchParams.get('q') || ''}
                            />
                        } />
                        <Route path="/people" element={
                            <PersonList
                                people={people}
                                onRefresh={handleRefreshPeople}
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
