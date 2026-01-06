import { useState, useEffect, useCallback } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import './GalleryDetail.css'
import './GalleryMetadata.css'
import ImageGrid from './ImageGrid'

function GalleryDetail() {
    const { id } = useParams()
    const navigate = useNavigate()
    const [gallery, setGallery] = useState(null)
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState(null)

    // Person Tagging State
    const [showPersonModal, setShowPersonModal] = useState(false)
    const [people, setPeople] = useState([])
    const [personSearchQuery, setPersonSearchQuery] = useState('')

    // Metadata Scraping State
    const [showMetadataModal, setShowMetadataModal] = useState(false)
    const [metadataResults, setMetadataResults] = useState([])
    const [metadataLoading, setMetadataLoading] = useState(false)
    const [scrapingMetadata, setScrapingMetadata] = useState(false)

    const fetchGallery = useCallback(async () => {
        setLoading(true)
        try {
            const response = await fetch(`/api/galleries/${id}`)
            if (!response.ok) throw new Error('Gallery not found')
            const data = await response.json()
            setGallery(data)
        } catch (err) {
            setError(err.message)
        } finally {
            setLoading(false)
        }
    }, [id])

    useEffect(() => {
        fetchGallery()
    }, [fetchGallery])

    const fetchPeople = async () => {
        try {
            const response = await fetch('/api/people?limit=1000')
            const data = await response.json()
            setPeople(data.data || [])
        } catch (error) {
            console.error('Failed to fetch people:', error)
        }
    }

    const handleDeleteGallery = async () => {
        const deleteImages = confirm(
            'Delete gallery images too?\n\nOK = Delete gallery AND images\nCancel = Delete gallery only (keep images)'
        )

        if (!confirm(`Are you sure you want to delete this gallery${deleteImages ? ' and all its images' : ''}?`)) {
            return
        }

        try {
            const url = `/api/galleries/${id}${deleteImages ? '?delete_images=true' : ''}`
            const response = await fetch(url, { method: 'DELETE' })

            if (response.ok) {
                navigate('/galleries')
            } else {
                alert('Failed to delete gallery')
            }
        } catch (error) {
            console.error('Error deleting gallery:', error)
            alert('Error deleting gallery')
        }
    }

    const handleRecrawlSource = async () => {
        if (!gallery || !gallery.source) return

        if (!confirm(`Re-crawl source "${gallery.source.name}"? This will fetch new content from the source.`)) {
            return
        }

        try {
            const response = await fetch(`/api/sources/${gallery.source.id}/crawl`, {
                method: 'POST'
            })

            if (response.ok) {
                alert('Source crawl started! New content will appear shortly.')
            } else {
                alert('Failed to start crawl')
            }
        } catch (error) {
            console.error('Error starting crawl:', error)
            alert('Error starting crawl')
        }
    }

    const handleTagPerson = async (personId) => {
        try {
            const response = await fetch(`/api/people/${personId}/galleries/${id}`, {
                method: 'POST'
            })
            if (response.ok) {
                fetchGallery()
                setShowPersonModal(false)
                setPersonSearchQuery('')
            } else {
                alert('Failed to tag person')
            }
        } catch (error) {
            console.error('Error tagging person:', error)
            alert('Error tagging person')
        }
    }

    const handleUntagPerson = async (personId) => {
        try {
            const response = await fetch(`/api/people/${personId}/galleries/${id}`, {
                method: 'DELETE'
            })
            if (response.ok) {
                fetchGallery()
            } else {
                alert('Failed to untag person')
            }
        } catch (error) {
            console.error('Error untagging person:', error)
            alert('Error untagging person')
        }
    }

    const handleSearchMetadata = async () => {
        setMetadataLoading(true)
        setShowMetadataModal(true)
        try {
            const response = await fetch(`/api/galleries/${id}/search-metadata`)
            if (!response.ok) throw new Error('Search failed')
            const data = await response.json()
            setMetadataResults(data.results || [])
        } catch (error) {
            console.error('Error searching metadata:', error)
            alert('Failed to search for gallery metadata')
            setShowMetadataModal(false)
        } finally {
            setMetadataLoading(false)
        }
    }

    const handleScrapeMetadata = async (sourceURL, provider, sourceID) => {
        setScrapingMetadata(true)
        try {
            const response = await fetch(`/api/galleries/${id}/scrape-metadata`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ source_url: sourceURL, provider, source_id: sourceID })
            })
            if (!response.ok) throw new Error('Scrape failed')
            const data = await response.json()
            alert('Gallery metadata updated successfully!')
            setShowMetadataModal(false)
            fetchGallery()
        } catch (error) {
            console.error('Error scraping metadata:', error)
            alert('Failed to scrape gallery metadata')
        } finally {
            setScrapingMetadata(false)
        }
    }

    if (loading) return <div className="loading">Loading...</div>
    if (error) return <div className="error">Error: {error}</div>
    if (!gallery) return <div className="error">Gallery not found</div>

    // Get first image for cover
    const coverImage = gallery.images && gallery.images.length > 0 ? gallery.images[0] : null

    return (
        <div className="gallery-detail">
            <button className="back-btn" onClick={() => navigate('/galleries')}>
                ← Back to Galleries
            </button>

            <div className="gallery-header-layout">
                {/* Left Column: Cover Info */}
                <div className="gallery-sidebar">
                    <div className="gallery-cover">
                        {coverImage ? (
                            <img
                                src={`/api/${coverImage.thumbnail_path}`}
                                alt={gallery.name}
                            />
                        ) : (
                            <div className="no-cover">No Images</div>
                        )}
                        <div className="gallery-meta-overlay">
                            <span className="image-count-badge">{gallery.images ? gallery.images.length : 0} Items</span>
                        </div>
                    </div>

                    <div className="gallery-actions">
                        <button onClick={handleSearchMetadata} className="action-btn primary full-width">
                            🔍 Fetch Metadata
                        </button>
                        {gallery.source && (
                            <button onClick={handleRecrawlSource} className="action-btn secondary full-width">
                                ↻ Re-crawl Source
                            </button>
                        )}
                        <button onClick={handleDeleteGallery} className="action-btn danger full-width">
                            🗑 Delete Gallery
                        </button>
                    </div>
                </div>

                {/* Right Column: Main Info */}
                <div className="gallery-main-info">
                    <div className="title-row">
                        <h1>{gallery.name}</h1>
                        <div className="gallery-badges">
                            {gallery.source && (
                                <span className="source-badge">
                                    {gallery.source.name}
                                </span>
                            )}
                            {gallery.provider && (
                                <span className="provider-badge-display">
                                    {gallery.provider}
                                </span>
                            )}
                            {gallery.rating > 0 && (
                                <span className="rating-badge">
                                    ★ {gallery.rating.toFixed(1)}
                                </span>
                            )}
                        </div>
                    </div>

                    <div className="gallery-metadata-section">
                        {gallery.release_date && (
                            <div className="meta-row">
                                <span className="meta-label">Released:</span>
                                <span className="meta-value">
                                    {new Date(gallery.release_date).toLocaleDateString(undefined, {
                                        year: 'numeric',
                                        month: 'long',
                                        day: 'numeric'
                                    })}
                                </span>
                            </div>
                        )}

                        {gallery.description && (
                            <div className="gallery-description">
                                {gallery.description}
                            </div>
                        )}
                    </div>

                    <div className="people-section">
                        <div className="section-header">
                            <h3>Performers</h3>
                            <button
                                className="add-person-btn"
                                onClick={() => {
                                    fetchPeople()
                                    setShowPersonModal(true)
                                }}
                            >
                                + Add Person
                            </button>
                        </div>

                        <div className="people-list">
                            {gallery.people && gallery.people.length > 0 ? (
                                gallery.people.map(person => (
                                    <div key={person.id} className="person-chip">
                                        <span onClick={() => navigate(`/people/${person.id}`)} className="person-name">
                                            {person.name}
                                        </span>
                                        <button onClick={() => handleUntagPerson(person.id)} className="remove-person">
                                            ✕
                                        </button>
                                    </div>
                                ))
                            ) : (
                                <p className="empty-text">No performers tagged</p>
                            )}
                        </div>
                    </div>
                </div>
            </div>

            <div className="gallery-content">
                <h2>Gallery Content</h2>
                <ImageGrid gallery={gallery} onRefresh={fetchGallery} />
            </div>

            {/* Person Tagging Modal */}
            {showPersonModal && (
                <div className="modal-overlay" onClick={() => setShowPersonModal(false)}>
                    <div className="modal-content" onClick={e => e.stopPropagation()}>
                        <h2>Tag Person</h2>
                        <input
                            type="text"
                            placeholder="Search people..."
                            value={personSearchQuery}
                            onChange={(e) => setPersonSearchQuery(e.target.value)}
                            className="search-input"
                            autoFocus
                        />
                        <div className="search-results-list">
                            {people
                                .filter(p =>
                                    p.name.toLowerCase().includes(personSearchQuery.toLowerCase()) &&
                                    !(gallery.people || []).find(tp => tp.id === p.id)
                                )
                                .slice(0, 10)
                                .map(person => (
                                    <div
                                        key={person.id}
                                        className="search-result-item"
                                        onClick={() => handleTagPerson(person.id)}
                                    >
                                        {person.name}
                                    </div>
                                ))
                            }
                        </div>
                        <button onClick={() => setShowPersonModal(false)} className="close-modal-btn">Close</button>
                    </div>
                </div>
            )}

            {/* Metadata Search Modal */}
            {showMetadataModal && (
                <div className="modal-overlay" onClick={() => !scrapingMetadata && setShowMetadataModal(false)}>
                    <div className="modal-content metadata-modal" onClick={e => e.stopPropagation()}>
                        <h2>Select Gallery to Scrape</h2>
                        {metadataLoading ? (
                            <div className="loading">Searching...</div>
                        ) : metadataResults.length === 0 ? (
                            <p>No matching galleries found</p>
                        ) : (
                            <div className="metadata-results">
                                {metadataResults.map((result, index) => (
                                    <div key={index} className="metadata-result-card">
                                        {result.thumbnail && (
                                            <img src={result.thumbnail} alt={result.title} className="result-thumbnail" />
                                        )}
                                        <div className="result-info">
                                            <span className="provider-badge">{result.provider}</span>
                                            <h3>{result.title}</h3>
                                            {result.release_date && <p className="release-date">{result.release_date}</p>}
                                        </div>
                                        <button
                                            onClick={() => handleScrapeMetadata(result.url, result.provider, result.source_id)}
                                            disabled={scrapingMetadata}
                                            className="action-btn primary"
                                        >
                                            {scrapingMetadata ? 'Scraping...' : 'Select'}
                                        </button>
                                    </div>
                                ))}
                            </div>
                        )}
                        <button
                            onClick={() => setShowMetadataModal(false)}
                            disabled={scrapingMetadata}
                            className="close-modal-btn"
                        >
                            Close
                        </button>
                    </div>
                </div>
            )}
        </div>
    )
}

export default GalleryDetail
