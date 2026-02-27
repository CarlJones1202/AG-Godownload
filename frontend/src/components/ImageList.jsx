import { useState } from 'react'
import './ImageList.css'
import Lightbox from './Lightbox'
import SortControls from './SortControls'

function ImageList({ images, onRefresh, meta, onPageChange, sort, setSort, seed, setSeed, onlyExisting, setOnlyExisting }) {
    const [lightboxImage, setLightboxImage] = useState(null)
    const [lightboxIndex, setLightboxIndex] = useState(0)

    const handleDeleteImage = async (imageId, e) => {
        e.stopPropagation()

        if (!confirm('Are you sure you want to delete this image?')) {
            return
        }

        try {
            const response = await fetch(`/api/images/${imageId}`, { method: 'DELETE' })

            if (response.ok) {
                onRefresh()
            } else {
                alert('Failed to delete image')
            }
        } catch (error) {
            console.error('Error deleting image:', error)
            alert('Error deleting image')
        }
    }

    const handleToggleFavorite = async (e, image) => {
        e.stopPropagation()
        try {
            const response = await fetch(`/api/images/${image.id}/favorite`, {
                method: 'POST'
            })
            if (response.ok) {
                onRefresh()
            }
        } catch (error) {
            console.error('Error toggling favorite:', error)
        }
    }

    const openLightbox = (image, index) => {
        setLightboxImage(image)
        setLightboxIndex(index)
    }

    const handleToggleOnlyExisting = (e) => {
        if (setOnlyExisting) {
            setOnlyExisting(e.target.checked)
        }
    }

    const closeLightbox = () => {
        setLightboxImage(null)
    }

    const nextImage = () => {
        const nextIndex = (lightboxIndex + 1) % images.length
        setLightboxIndex(nextIndex)
        setLightboxImage(images[nextIndex])
    }

    const prevImage = () => {
        const prevIndex = (lightboxIndex - 1 + images.length) % images.length
        setLightboxIndex(prevIndex)
        setLightboxImage(images[prevIndex])
    }

    return (
        <div className="image-list">
            <div className="image-header">
                <h2>All Images</h2>
                <div className="header-actions">
                    {setOnlyExisting && (
                        <label className="checkbox-label">
                            <input
                                type="checkbox"
                                checked={onlyExisting}
                                onChange={handleToggleOnlyExisting}
                            />
                            Only Existing
                        </label>
                    )}
                    <SortControls
                        sort={sort}
                        setSort={setSort}
                        seed={seed}
                        setSeed={setSeed}
                        onRandomize={setSeed}
                    />
                    <button onClick={onRefresh}>🔄 Refresh</button>
                </div>
            </div>

            {images.length === 0 ? (
                <div className="no-images">
                    <p>No images found. Add a source to start crawling!</p>
                </div>
            ) : (
                <div className="images-grid">
                    {images.map((image, index) => (
                        <div
                            key={image.id}
                            className="image-card"
                            onClick={() => openLightbox(image, index)}
                        >
                            <div className="image-thumbnail">
                                <img
                                    src={image.thumbnail_path || `/api/thumbnails/${image.filename}`}
                                    alt={image.filename}
                                    loading="lazy"
                                />
                            </div>
                            {/* <div className="image-info">
                                <p className="gallery-name">{image.gallery?.name || 'Unknown'}</p>
                                <p className="image-filename">{image.filename}</p>
                            </div> */}
                            <button
                                className={`favorite-btn ${image.is_favorite ? 'active' : ''}`}
                                onClick={(e) => handleToggleFavorite(e, image)}
                                title={image.is_favorite ? "Unfavorite" : "Favorite"}
                            >
                                <svg width="16" height="16" viewBox="0 0 24 24" fill={image.is_favorite ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                    <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"></path>
                                </svg>
                            </button>
                            <button
                                className="delete-image-btn"
                                onClick={(e) => handleDeleteImage(image.id, e)}
                                title="Delete Image"
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

            {lightboxImage && (
                <Lightbox
                    image={lightboxImage}
                    images={images}
                    currentIndex={lightboxIndex}
                    totalImages={images.length}
                    onClose={closeLightbox}
                    onNext={nextImage}
                    onPrev={prevImage}
                />
            )}
        </div>
    )
}

export default ImageList
