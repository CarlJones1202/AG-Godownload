import { useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import './GalleryList.css'
import './PersonTagging.css'
import './GalleryList_recrawl.css'

function GalleryList({ galleries, onRefresh, meta, onPageChange }) {
    const navigate = useNavigate()
    const location = useLocation()

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

    const handleSelectGallery = (gallery) => {
        navigate(`/galleries/${gallery.id}`)
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
