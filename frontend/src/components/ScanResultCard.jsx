// frontend/src/components/ScanResultCard.jsx
import React from 'react'

export default function ScanResultCard({
  gallery,
  provider,
  onLink,
  onReject,
  isLinking,
  isExcluding,
  titleLines = 2
}) {
  return (
    <a
      href={gallery.url}
      target="_blank"
      rel="noopener noreferrer"
      className="scan-result-card"
    >
      <div className="gallery-thumbnail">
        {gallery.thumbnail ? (
          <img
            src={gallery.thumbnail}
            alt={gallery.title}
            loading="lazy"
            onError={(e) => {
              e.target.onerror = null;
              e.target.style.display = 'none';
              e.target.parentNode.classList.add('image-error');
            }}
          />
        ) : (
          <div className="no-image">No Thumbnail</div>
        )}
      </div>

      <div className="gallery-info">
        {(provider || gallery.provider) && (
          <div className="gallery-provider">{provider || gallery.provider}</div>
        )}
        <h3 style={{ WebkitLineClamp: titleLines }}>{gallery.title}</h3>
        {gallery.release_date && (
          <p className="gallery-meta">{gallery.release_date}</p>
        )}
      </div>

      <div className="scan-result-actions">
        {onLink && (
          <button
            className="action-btn-small action-btn-link"
            aria-label="Link gallery"
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); onLink(gallery) }}
            disabled={isLinking}
            title="Link to this person"
          >
            {isLinking ? '…' : '→'}
          </button>
        )}

        {onReject && (
          <button
            className="action-btn-small action-btn-reject"
            aria-label="Reject gallery"
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); onReject(gallery) }}
            disabled={isExcluding}
            title="Reject this result"
          >
            {isExcluding ? '…' : '✕'}
          </button>
        )}
      </div>
    </a>
  )
}