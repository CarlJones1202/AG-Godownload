import { useState, useEffect } from 'react'
import './AutoTag.css'

const SOURCES = ['MetArt', 'MetartX', 'Playboy', 'PlayboyPlus', 'Vixen', 'SexArt', 'LifeErotic', 'EternalDesire', 'MPLStudios', 'VivThomas', 'WowGirls']

function SourceScanModal({ show, onClose, personId, personName, personAliases, onScanComplete }) {
    const [selectedSource, setSelectedSource] = useState('MetArt')
    const [selectedAlias, setSelectedAlias] = useState('')
    const [scanning, setScanning] = useState(false)
    const [results, setResults] = useState(null)
    const [error, setError] = useState(null)
    const [linking, setLinking] = useState(null)
    const [linkedGalleries, setLinkedGalleries] = useState({})

    useEffect(() => {
        if (show) {
            setSelectedAlias('')
            setResults(null)
            setError(null)
            setLinkedGalleries({})
        }
    }, [show])

    const aliases = personAliases ? (Array.isArray(personAliases) ? personAliases : JSON.parse(personAliases)) : []

    const handleScan = async () => {
        setScanning(true)
        setError(null)
        setResults(null)
        setLinkedGalleries({})

        try {
            const params = new URLSearchParams({
                source: selectedSource
            })
            if (selectedAlias) {
                params.append('alias', selectedAlias)
            }
            
            const response = await fetch(`/api/people/${personId}/scan?${params}`)
            const data = await response.json()

            if (!response.ok) {
                throw new Error(data.error || 'Scan failed')
            }

            setResults(data)
        } catch (err) {
            setError(err.message)
        } finally {
            setScanning(false)
        }
    }

    const handleLinkGallery = async (gallery) => {
        if (!confirm(`Link this gallery to ${selectedSource}? This will set the provider to ${selectedSource}.`)) {
            return
        }

        setLinking(gallery.title)
        try {
            const response = await fetch(`/api/galleries/${gallery.id}/update-provider`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    provider: selectedSource,
                    source_url: gallery.url
                })
            })

            if (!response.ok) {
                throw new Error('Failed to link gallery')
            }

            setLinkedGalleries(prev => ({
                ...prev,
                [gallery.title]: true
            }))

            if (onScanComplete) {
                onScanComplete()
            }
        } catch (err) {
            alert(err.message)
        } finally {
            setLinking(null)
        }
    }

    if (!show) return null

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content scan-modal" onClick={e => e.stopPropagation()}>
                <h2>Scan Source for {personName}</h2>

                <div className="scan-controls">
                    <label>Source:</label>
                    <select
                        value={selectedSource}
                        onChange={(e) => setSelectedSource(e.target.value)}
                        disabled={scanning}
                    >
                        {SOURCES.map(source => (
                            <option key={source} value={source}>{source}</option>
                        ))}
                    </select>
                    {aliases.length > 0 && (
                        <>
                            <label>Alias:</label>
                            <select
                                value={selectedAlias}
                                onChange={(e) => setSelectedAlias(e.target.value)}
                                disabled={scanning}
                            >
                                <option value="">All (name + aliases)</option>
                                <option value={personName}>{personName}</option>
                                {aliases.map(alias => (
                                    <option key={alias} value={alias}>{alias}</option>
                                ))}
                            </select>
                        </>
                    )}
                    <button
                        onClick={handleScan}
                        disabled={scanning}
                        className="scan-btn"
                    >
                        {scanning ? 'Scanning...' : 'Scan'}
                    </button>
                </div>

                {error && (
                    <div className="scan-error">
                        {error}
                    </div>
                )}

                {results && (
                    <div className="scan-results">
                        <div className="scan-summary">
                            <span className="summary-item">
                                <strong>Found:</strong> {results.found_count}
                            </span>
                            <span className="summary-item existing">
                                <strong>Existing:</strong> {results.existing_count}
                            </span>
                            {results.unsure_count > 0 && (
                                <span className="summary-item unsure">
                                    <strong>Needs Review:</strong> {results.unsure_count}
                                </span>
                            )}
                            <span className="summary-item missing">
                                <strong>Missing:</strong> {results.missing_count}
                            </span>
                        </div>

                        {results.unsure_galleries && results.unsure_galleries.length > 0 && (
                            <div className="unsure-galleries">
                                <h3>Needs Review</h3>
                                <p className="help-text">These galleries exist in your database but don't have a provider set. Click "Link" to assign them to {selectedSource}.</p>
                                <div className="scan-gallery-list">
                                    {results.unsure_galleries.map((gallery, index) => (
                                        <div key={index} className="gallery-item">
                                            <div className="gallery-thumb">
                                                {gallery.thumbnail ? (
                                                    <img src={gallery.thumbnail} alt={gallery.title} />
                                                ) : (
                                                    <div className="no-thumb">No Image</div>
                                                )}
                                            </div>
                                            <div className="gallery-info">
                                                <span className="gallery-title">{gallery.title}</span>
                                                {gallery.release_date && (
                                                    <span className="gallery-date">{gallery.release_date}</span>
                                                )}
                                            </div>
                                            {gallery.id ? (
                                                <button
                                                    onClick={() => handleLinkGallery(gallery)}
                                                    disabled={linking === gallery.title || linkedGalleries[gallery.title]}
                                                    className="link-btn"
                                                >
                                                    {linkedGalleries[gallery.title] ? 'Linked!' : linking === gallery.title ? 'Linking...' : 'Link'}
                                                </button>
                                            ) : (
                                                <a
                                                    href={gallery.url}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="view-btn"
                                                >
                                                    View
                                                </a>
                                            )}
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}

                        {results.missing_galleries && results.missing_galleries.length > 0 ? (
                            <div className="missing-galleries">
                                <h3>Missing Galleries</h3>
                                <div className="scan-gallery-list">
                                    {results.missing_galleries.map((gallery, index) => (
                                        <div key={index} className="gallery-item">
                                            <div className="gallery-thumb">
                                                {gallery.thumbnail ? (
                                                    <img src={gallery.thumbnail} alt={gallery.title} />
                                                ) : (
                                                    <div className="no-thumb">No Image</div>
                                                )}
                                            </div>
                                            <div className="gallery-info">
                                                <a
                                                    href={gallery.url}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="gallery-title"
                                                >
                                                    {gallery.title}
                                                </a>
                                                {gallery.release_date && (
                                                    <span className="gallery-date">{gallery.release_date}</span>
                                                )}
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        ) : results.unsure_galleries?.length === 0 ? (
                            <div className="no-missing">
                                No missing galleries found on {results.provider}.
                            </div>
                        ) : null}
                    </div>
                )}

                <button className="close-modal-btn" onClick={onClose}>Close</button>
            </div>
        </div>
    )
}

export default SourceScanModal
