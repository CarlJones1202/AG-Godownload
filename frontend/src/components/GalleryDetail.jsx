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
    const [manualUrl, setManualUrl] = useState('')

    // Name Editing State
    const [isEditingName, setIsEditingName] = useState(false)
    const [editedName, setEditedName] = useState('')

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

    const handleUpdateName = async () => {
        if (!editedName.trim() || editedName === gallery.name) {
            setIsEditingName(false)
            return
        }

        try {
            const response = await fetch(`/api/galleries/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: editedName })
            })

            if (response.ok) {
                setGallery(prev => ({ ...prev, name: editedName }))
                setIsEditingName(false)
            } else {
                alert('Failed to update gallery name')
            }
        } catch (error) {
            console.error('Error updating gallery name:', error)
            alert('Error updating gallery name')
        }
    }

    const startEditing = () => {
        setEditedName(gallery.name)
        setIsEditingName(true)
    }

    const handleManualScrape = () => {
        if (!manualUrl.trim()) return

        let provider = 'MetArt' // Default
        const lowerUrl = manualUrl.toLowerCase()
        if (lowerUrl.includes('playboy.com')) {
            provider = 'Playboy'
        } else if (lowerUrl.includes('playboyplus.com')) {
            provider = 'PlayboyPlus'
        } else if (lowerUrl.includes('vixen.com')) {
            provider = 'Vixen'
        } else if (lowerUrl.includes('metart.com')) {
            provider = 'MetArt'
        }

        handleScrapeMetadata(manualUrl, provider, null)
    }

    if (loading) return <div className="loading">Loading...</div>
    if (error) return <div className="error">Error: {error}</div>
    if (!gallery) return <div className="error">Gallery not found</div>

    // Get first image for cover
    const coverImage = gallery.images && gallery.images.length > 0 ? gallery.images[0] : null

    return (
        <div className="gallery-detail">
            <a href="/galleries" className="back-btn" onClick={(e) => {
                e.preventDefault();
                navigate('/galleries');
            }}>
                ← Back to Galleries
            </a>

            <div className="gallery-header-layout">
                {/* Left Column: Cover Info */}
                <div className="gallery-sidebar">
                    <div className="gallery-cover">
                        {coverImage ? (
                            <img
                                src={`/api/images/${coverImage.filename}`}
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
                    <div className="premium-header">
                        {/* Eyebrow: Provider Only */}
                        <div className="header-eyebrow">
                            {gallery.provider && (
                                <span className="eyebrow-badge provider">
                                    {gallery.provider}
                                </span>
                            )}
                        </div>


                        {/* Main Title */}
                        <div className="title-section">
                            {isEditingName ? (
                                <div className="edit-name-container">
                                    <input
                                        type="text"
                                        value={editedName}
                                        onChange={(e) => setEditedName(e.target.value)}
                                        className="edit-name-input"
                                        autoFocus
                                        onKeyDown={(e) => {
                                            if (e.key === 'Enter') handleUpdateName()
                                            if (e.key === 'Escape') setIsEditingName(false)
                                        }}
                                    />
                                    <button onClick={handleUpdateName} className="save-btn" title="Save">✓</button>
                                    <button onClick={() => setIsEditingName(false)} className="cancel-btn" title="Cancel">✕</button>
                                </div>
                            ) : (
                                <div className="title-row">
                                    <h1 className="premium-title">{gallery.name}</h1>
                                    <button onClick={startEditing} className="edit-title-btn" title="Edit Name">✎</button>
                                </div>
                            )}
                        </div>

                        {/* Metadata Strip: Date, Rating, Count */}
                        <div className="meta-strip">
                            {gallery.rating > 0 && (
                                <div className="meta-item rating">
                                    <span className="star-icon">★</span>
                                    <span className="rating-value">{gallery.rating.toFixed(1)}</span>
                                </div>
                            )}

                            {gallery.release_date && (
                                <>
                                    <div className="meta-divider">•</div>
                                    <div className="meta-item date">
                                        {new Date(gallery.release_date).toLocaleDateString(undefined, {
                                            year: 'numeric',
                                            month: 'long',
                                            day: 'numeric'
                                        })}
                                    </div>
                                </>
                            )}

                            <div className="meta-divider">•</div>
                            <div className="meta-item count">
                                {gallery.images ? gallery.images.length : 0} Images
                            </div>
                        </div>

                        {/* Description */}
                        {gallery.description && (
                            <div className="premium-description">
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
                                        <a
                                            href={`/people/${person.id}`}
                                            className="person-name"
                                            onClick={(e) => {
                                                if (e.button === 0 && !e.ctrlKey && !e.metaKey) {
                                                    e.preventDefault();
                                                    navigate(`/people/${person.id}`);
                                                }
                                            }}
                                        >
                                            {person.name}
                                        </a>
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
                        <h2>Fetch Metadata</h2>

                        <div className="manual-link-section">
                            <p className="modal-subtitle">Paste a gallery URL directly or select from search results below.</p>
                            <div className="manual-link-input-group">
                                <input
                                    type="text"
                                    placeholder="https://www.metart.com/..."
                                    value={manualUrl}
                                    onChange={(e) => setManualUrl(e.target.value)}
                                    className="manual-url-input"
                                />
                                <button
                                    onClick={handleManualScrape}
                                    disabled={!manualUrl || scrapingMetadata}
                                    className="action-btn primary"
                                >
                                    Use URL
                                </button>
                            </div>
                        </div>

                        <div className="search-results-divider">
                            <span>OR Select Result</span>
                        </div>

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
