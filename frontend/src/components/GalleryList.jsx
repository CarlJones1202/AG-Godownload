import { useState, useEffect } from 'react'
import { useNavigate, useParams, useLocation } from 'react-router-dom'
import './GalleryList.css'
import './PersonTagging.css'
import './GalleryList_recrawl.css'
import ImageGrid from './ImageGrid'

function GalleryList({ galleries, onRefresh, meta, onPageChange }) {
    const [selectedGallery, setSelectedGallery] = useState(null)
    const [showPersonModal, setShowPersonModal] = useState(false)
    const [people, setPeople] = useState([])
    const [searchQuery, setSearchQuery] = useState('')
    const [sidebarOpen, setSidebarOpen] = useState(false)
    const navigate = useNavigate()
    const params = useParams()
    const location = useLocation()

    // Check if we're in "view gallery" mode based on URL
    const galleryId = params.id

    useEffect(() => {
        if (galleryId) {
            fetchGalleryDetails(galleryId)
        } else {
            setSelectedGallery(null)
        }
    }, [galleryId])

    const fetchGalleryDetails = async (id) => {
        try {
            const response = await fetch(`/api/galleries/${id}`)
            if (response.ok) {
                const fullGallery = await response.json()
                setSelectedGallery(fullGallery)
            }
        } catch (error) {
            console.error('Error fetching gallery:', error)
        }
    }

    const handleDeleteGallery = async (galleryId, e) => {
        e.stopPropagation()

        const deleteImages = confirm(
            'Delete gallery images too?\n\nOK = Delete gallery AND images\nCancel = Delete gallery only (keep images)'
        )

        if (!confirm(`Are you sure you want to delete this gallery${deleteImages ? ' and all its images' : ''}?`)) {
            return
        }

        try {
            const url = `/api/galleries/${galleryId}${deleteImages ? '?delete_images=true' : ''}`
            const response = await fetch(url, { method: 'DELETE' })

            if (response.ok) {
                onRefresh()
            } else {
                alert('Failed to delete gallery')
            }
        } catch (error) {
            console.error('Error deleting gallery:', error)
            alert('Error deleting gallery')
        }
    }

    const handleRefreshGallery = async () => {
        if (selectedGallery) {
            fetchGalleryDetails(selectedGallery.id)
        }
        onRefresh()
    }

    const handleSelectGallery = (gallery) => {
        navigate(`/galleries/${gallery.id}`)
    }

    // Fetch people list for tagging
    const fetchPeople = async () => {
        try {
            const response = await fetch('/api/people?limit=1000')
            const data = await response.json()
            setPeople(data.data || [])
        } catch (error) {
            console.error('Failed to fetch people:', error)
        }
    }

    const handleTagPerson = async (personId) => {
        if (!selectedGallery) return
        try {
            const response = await fetch(`/api/people/${personId}/galleries/${selectedGallery.id}`, {
                method: 'POST'
            })
            if (response.ok) {
                // Refresh gallery to get updated people list
                fetchGalleryDetails(selectedGallery.id)
                setShowPersonModal(false)
                setSearchQuery('')
            } else {
                alert('Failed to tag person')
            }
        } catch (error) {
            console.error('Error tagging person:', error)
            alert('Error tagging person')
        }
    }

    const handleUntagPerson = async (personId) => {
        if (!selectedGallery) return
        try {
            const response = await fetch(`/api/people/${personId}/galleries/${selectedGallery.id}`, {
                method: 'DELETE'
            })
            if (response.ok) {
                // Refresh gallery to get updated people list
                fetchGalleryDetails(selectedGallery.id)
            } else {
                alert('Failed to untag person')
            }
        } catch (error) {
            console.error('Error untagging person:', error)
            alert('Error untagging person')
        }
    }

    const handleExcludePerson = async (personId) => {
        if (!selectedGallery) return

        if (!confirm('Mark this gallery as NOT featuring this person? This will prevent auto-tagging.')) {
            return
        }

        try {
            const response = await fetch(`/api/people/${personId}/exclude-gallery/${selectedGallery.id}`, {
                method: 'POST'
            })

            if (response.ok) {
                fetchGalleryDetails(selectedGallery.id)
                alert('Gallery excluded from this person')
            } else {
                alert('Failed to exclude gallery')
            }
        } catch (error) {
            console.error('Error excluding gallery:', error)
            alert('Error excluding gallery')
        }
    }

    const handleRecrawlSource = async () => {
        if (!selectedGallery || !selectedGallery.source) return

        if (!confirm(`Re-crawl source "${selectedGallery.source.name}"? This will fetch new content from the source.`)) {
            return
        }

        try {
            const response = await fetch(`/api/sources/${selectedGallery.source.id}/crawl`, {
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

    if (selectedGallery) {

        return (
            <div>
                <button
                    className="back-button"
                    onClick={() => navigate('/galleries')}
                >
                    ← Back to Galleries
                </button>

                {/* Clean Title Header */}
                <div className="gallery-title-header">
                    <h2>{selectedGallery.name}</h2>
                    {selectedGallery.source && (
                        <span className="source-badge">{selectedGallery.source.name}</span>
                    )}
                </div>

                {/* Floating Action Button */}
                <button
                    className="fab-button"
                    onClick={() => setSidebarOpen(!sidebarOpen)}
                    title="Gallery Actions"
                >
                    ⋮
                </button>

                {/* Slide-out Sidebar */}
                <div className={`admin-sidebar ${sidebarOpen ? 'open' : ''}`}>
                    <div className="sidebar-header">
                        <h3>Actions</h3>
                        <button className="close-sidebar" onClick={() => setSidebarOpen(false)}>✕</button>
                    </div>
                    <div className="sidebar-content">
                        <button
                            onClick={() => {
                                fetchPeople()
                                setShowPersonModal(true)
                            }}
                            className="sidebar-action-btn"
                        >
                            <div className="btn-text">
                                <strong>Tag Person</strong>
                                <small>Link people to this gallery</small>
                            </div>
                        </button>
                        {selectedGallery.source && (
                            <button
                                className="sidebar-action-btn"
                                onClick={handleRecrawlSource}
                            >
                                <div className="btn-text">
                                    <strong>Re-crawl Source</strong>
                                    <small>Fetch new content from {selectedGallery.source.name}</small>
                                </div>
                            </button>
                        )}
                        <button
                            className="sidebar-action-btn danger"
                            onClick={(e) => handleDeleteGallery(selectedGallery.id, e)}
                        >
                            <div className="btn-text">
                                <strong>Delete Gallery</strong>
                                <small>Remove gallery and optionally images</small>
                            </div>
                        </button>
                    </div>
                </div>

                {/* Overlay */}
                {sidebarOpen && <div className="sidebar-overlay" onClick={() => setSidebarOpen(false)} />}
                {/* Tagged People Display */}
                {selectedGallery.people && selectedGallery.people.length > 0 && (
                    <div className="gallery-tagged-people">
                        <h3>Tagged People:</h3>
                        <div className="tagged-people-list">
                            {selectedGallery.people.map(person => (
                                <div key={person.id} className="tagged-person-chip">
                                    <span onClick={() => navigate(`/people/${person.id}`)} style={{ cursor: 'pointer' }}>
                                        {person.name}
                                    </span>
                                    <button onClick={() => handleUntagPerson(person.id)} title="Remove tag">✕</button>
                                </div>
                            ))}
                        </div>
                    </div>
                )}
                <ImageGrid gallery={selectedGallery} onRefresh={handleRefreshGallery} />

                {/* Person Tagging Modal */}
                {showPersonModal && (
                    <div className="person-modal-overlay" onClick={() => setShowPersonModal(false)}>
                        <div className="person-modal" onClick={e => e.stopPropagation()}>
                            <h3>Tag Person</h3>
                            <input
                                type="text"
                                placeholder="Search people..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="person-search"
                            />
                            <div className="tagging-person-list">
                                {people
                                    .filter(p =>
                                        p.name.toLowerCase().includes(searchQuery.toLowerCase()) &&
                                        !(selectedGallery.people || []).find(tp => tp.id === p.id)
                                    )
                                    .map(person => (
                                        <div
                                            key={person.id}
                                            className="person-item"
                                            onClick={() => handleTagPerson(person.id)}
                                        >
                                            {person.name}
                                        </div>
                                    ))
                                }
                            </div>
                            <button onClick={() => setShowPersonModal(false)} className="close-modal">Close</button>
                        </div>
                    </div>
                )}
            </div>
        )
    }

    return (
        <div className="gallery-list">
            {galleries.length === 0 ? (
                <div className="no-galleries">
                    <p>No galleries found. Add a source to start crawling!</p>
                </div>
            ) : (
                <div className="gallery-grid">
                    {galleries.map((gallery) => (
                        <div
                            key={gallery.id}
                            className="gallery-card"
                            onClick={() => handleSelectGallery(gallery)}
                        >
                            <div className="gallery-thumbnail">
                                {gallery.images && gallery.images.length > 0 ? (
                                    <img
                                        src={`/api/${gallery.images[0].thumbnail_path}`}
                                        alt={gallery.name}
                                        loading="lazy"
                                    />
                                ) : (
                                    <div className="no-image">No Images</div>
                                )}
                            </div>
                            <div className="gallery-info">
                                <h3>{gallery.name}</h3>
                                <p>{gallery.image_count || 0} images</p>
                            </div>
                            <button
                                className="delete-gallery-card-btn"
                                onClick={(e) => handleDeleteGallery(gallery.id, e)}
                                title="Delete Gallery"
                            >
                                🗑️
                            </button>
                        </div>
                    ))}
                </div>
            )}

            {meta && meta.total_pages > 1 && (
                <div className="pagination">
                    <button
                        disabled={meta.current_page === 1}
                        onClick={() => onPageChange(meta.current_page - 1)}
                    >
                        Previous
                    </button>
                    <span>Page {meta.current_page} of {meta.total_pages}</span>
                    <button
                        disabled={meta.current_page === meta.total_pages}
                        onClick={() => onPageChange(meta.current_page + 1)}
                    >
                        Next
                    </button>
                </div>
            )}
        </div>
    )
}

export default GalleryList
