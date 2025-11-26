import { useState, useEffect } from 'react'
import { useNavigate, useParams, useLocation } from 'react-router-dom'
import './GalleryList.css'
import ImageGrid from './ImageGrid'

function GalleryList({ galleries, onRefresh, meta, onPageChange }) {
    const [selectedGallery, setSelectedGallery] = useState(null)
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

    if (selectedGallery) {
        return (
            <div>
                <button
                    className="back-button"
                    onClick={() => navigate('/galleries')}
                >
                    ← Back to Galleries
                </button>
                <div className="gallery-header">
                    <h2>{selectedGallery.name}</h2>
                    <button
                        className="delete-gallery-btn"
                        onClick={(e) => handleDeleteGallery(selectedGallery.id, e)}
                    >
                        🗑️ Delete Gallery
                    </button>
                </div>
                <ImageGrid gallery={selectedGallery} onRefresh={handleRefreshGallery} />
            </div>
        )
    }

    return (
        <div className="gallery-list">
            <div className="gallery-header">
                <h2>Your Galleries</h2>
                <button onClick={onRefresh}>🔄 Refresh</button>
            </div>
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
                                        src={`/api/thumbnails/${gallery.images[0].filename}`}
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
