import { useState } from 'react'
import './GalleryList.css'
import ImageGrid from './ImageGrid'

function GalleryList({ galleries, onRefresh }) {
    const [selectedGallery, setSelectedGallery] = useState(null)

    if (selectedGallery) {
        return (
            <div>
                <button
                    className="back-button"
                    onClick={() => setSelectedGallery(null)}
                >
                    ← Back to Galleries
                </button>
                <h2>{selectedGallery.name}</h2>
                <ImageGrid gallery={selectedGallery} />
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
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}

export default GalleryList
