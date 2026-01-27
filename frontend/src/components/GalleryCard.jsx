import React from 'react'
import './GalleryList.css'

function GalleryCard({ gallery, onClick, actionNode, showProvider = true }) {
    // Determine if it's a video based on first image type
    const isVideo = gallery.images && gallery.images.length > 0 && gallery.images[0].type === 'video'

    // Safety check for thumbnail path
    const thumbnailPath = gallery.images && gallery.images.length > 0
        ? `/api/${gallery.images[0].thumbnail_path}`
        : null

    return (
        <a
            href={`/galleries/${gallery.id}`}
            className="gallery-card"
            onClick={(e) => {
                // Only prevent default for regular left-clicks
                // Allow middle-click, Ctrl+click, Cmd+click to open in new tab
                if (e.button === 0 && !e.ctrlKey && !e.metaKey && onClick) {
                    e.preventDefault();
                    onClick();
                }
            }}
        >
            <div className={`gallery-thumbnail ${isVideo ? 'video-thumbnail' : ''}`}>
                {thumbnailPath ? (
                    <div className={`gallery-thumb-inner ${isVideo ? "video-thumb-container" : ""}`}>
                        <img
                            src={thumbnailPath}
                            alt={gallery.name}
                            loading="lazy"
                            onError={(e) => {
                                e.target.onerror = null;
                                // Fallback for broken images
                                e.target.style.display = 'none';
                                e.target.parentNode.classList.add('image-error');
                            }}
                        />
                        {isVideo && <div className="play-icon-overlay">▶</div>}
                    </div>
                ) : (
                    <div className="no-image">{isVideo ? 'No Video' : 'No Images'}</div>
                )}
            </div>

            <div className="gallery-info">
                {showProvider && gallery.provider && (
                    <div className="gallery-provider">{gallery.provider}</div>
                )}
                <h3>{gallery.name}</h3>
                <p className="image-count">{gallery.image_count || 0} images</p>
            </div>

            {actionNode}
        </a>
    )
}

export default GalleryCard
