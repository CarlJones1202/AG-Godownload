import { useState } from 'react'
import './GalleryList.css'
import ImageGrid from './ImageGrid'

function GalleryList({ galleries, onRefresh }) {
    const [selectedGallery, setSelectedGallery] = useState(null)

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

    const handleRefreshGallery = () => {
        onRefresh()
        // Re-fetch the selected gallery
        const updated = galleries.find(g => g.id === selectedGallery.id)
        if (updated) {
            setSelectedGallery(updated)
        }
    }

    if (selectedGallery) {
        return (
            <div>
                <button
                    className="back-button"
                    onClick={() => setSelectedGallery(null)}
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
                <div className="empty-state">
                    <p>No galleries yet. Add a source to get started!</p>
                </div>
            ) : (
                <div className="gallery-grid">
                    {galleries.map(gallery => (
                        <div
                            key={gallery.id}
                            className="gallery-card"
                            onClick={() => setSelectedGallery(gallery)}
                        >
                            <div className="gallery-thumbnail">
                                {gallery.images && gallery.images.length > 0 ? (
                                    <img
                                        src={`/api/thumbnails/${gallery.images[0].filename}`}
                                        alt={gallery.name}
                                    />
                                ) : (
                                    <div className="no-image">📁</div>
                                )}
                            </div>
                            <div className="gallery-info">
                                <h3>{gallery.name}</h3>
                                <p>{gallery.images?.length || 0} images</p>
                            </div>
                            <button
                                className="delete-gallery-card-btn"
                                onClick={(e) => handleDeleteGallery(gallery.id, e)}
                                title="Delete gallery"
                            >
                                ×
                            </button>
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}

export default GalleryList
