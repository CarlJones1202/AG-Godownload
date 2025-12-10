import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import './PersonDetail.css'
import './PersonDetail_identifiers.css'
import './AutoTag.css'
import GalleryList from './GalleryList'
import AutoTagModal from './AutoTagModal'

function PersonDetail() {
    const { id } = useParams()
    const navigate = useNavigate()
    const [person, setPerson] = useState(null)
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState(null)
    const [isEditing, setIsEditing] = useState(false)
    const [editForm, setEditForm] = useState({ name: '', aliases: '' })
    const [currentPhotoIndex, setCurrentPhotoIndex] = useState(0)

    const fetchPerson = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const response = await fetch(`/api/people/${id}`)
            if (!response.ok) {
                throw new Error(`Failed to fetch person: ${response.status}`)
            }
            const data = await response.json()
            setPerson(data)
            // Initialize edit form
            const aliases = parseAliases(data.aliases)
            setEditForm({
                name: data.name,
                aliases: aliases.join(', ')
            })
        } catch (error) {
            console.error('Failed to fetch person:', error)
            setError(error.message)
        } finally {
            setLoading(false)
        }
    }, [id])

    useEffect(() => {
        fetchPerson()
    }, [fetchPerson])

    const parseAliases = (aliasesStr) => {
        try {
            return JSON.parse(aliasesStr || '[]')
        } catch {
            return []
        }
    }

    const handleSaveEdit = async () => {
        const aliases = editForm.aliases
            .split(',')
            .map(a => a.trim())
            .filter(a => a.length > 0)

        try {
            const response = await fetch(`/api/people/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: editForm.name,
                    aliases: aliases
                })
            })

            if (response.ok) {
                setIsEditing(false)
                fetchPerson()
            } else {
                alert('Failed to update person')
            }
        } catch (error) {
            console.error('Error updating person:', error)
            alert('Error updating person')
        }
    }

    const [identifierSearch, setIdentifierSearch] = useState('')
    const [identifierResults, setIdentifierResults] = useState([])
    const [showIdentifierModal, setShowIdentifierModal] = useState(false)
    const [searchingIdentifier, setSearchingIdentifier] = useState(false)
    const [availableSources, setAvailableSources] = useState([])
    const [selectedSource, setSelectedSource] = useState('stashdb')

    // Auto-tag state
    const [showAutoTagModal, setShowAutoTagModal] = useState(false)
    const [autoTagSuggestions, setAutoTagSuggestions] = useState([])
    const [autoTagging, setAutoTagging] = useState(false)
    const [selectedSuggestions, setSelectedSuggestions] = useState(new Set())
    const [exclusions, setExclusions] = useState([])

    // Fetch available identifier sources
    useEffect(() => {
        fetch('/api/identifiers/sources')
            .then(res => res.json())
            .then(data => {
                if (data.sources) {
                    setAvailableSources(data.sources)
                }
            })
            .catch(err => console.error('Failed to fetch identifier sources:', err))
    }, [])

    const handleSearchIdentifier = async () => {
        if (!identifierSearch.trim()) return
        setSearchingIdentifier(true)
        try {
            const response = await fetch(`/api/identifiers/${selectedSource}/search?name=${encodeURIComponent(identifierSearch)}`)
            const result = await response.json()
            if (result.data) {
                setIdentifierResults(result.data)
            }
        } catch (error) {
            console.error(`Failed to search ${selectedSource}:`, error)
            alert(`Failed to search ${selectedSource}`)
        } finally {
            setSearchingIdentifier(false)
        }
    }

    const handleLinkIdentifier = async (externalId) => {
        try {
            const response = await fetch(`/api/people/${id}/identifiers`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    source: selectedSource,
                    external_id: externalId
                })
            })

            if (response.ok) {
                setShowIdentifierModal(false)
                setIdentifierResults([])
                setIdentifierSearch('')
                fetchPerson()
                alert(`Successfully linked to ${selectedSource}!`)
            } else {
                alert(`Failed to link to ${selectedSource}`)
            }
        } catch (error) {
            console.error(`Failed to link ${selectedSource}:`, error)
            alert(`Failed to link ${selectedSource}`)
        }
    }

    const handleUnlinkIdentifier = async (identifierId) => {
        if (!confirm('Are you sure you want to remove this identifier?')) return

        try {
            const response = await fetch(`/api/people/${id}/identifiers/${identifierId}`, {
                method: 'DELETE'
            })

            if (response.ok) {
                fetchPerson()
            } else {
                alert('Failed to remove identifier')
            }
        } catch (error) {
            console.error('Error removing identifier:', error)
            alert('Error removing identifier')
        }
    }

    const handleUnlinkGallery = async (galleryId, e) => {
        e.stopPropagation()

        if (!confirm('Are you sure you want to unlink this gallery?')) {
            return
        }

        try {
            const response = await fetch(`/api/people/${id}/galleries/${galleryId}`, {
                method: 'DELETE'
            })

            if (response.ok) {
                fetchPerson()
            } else {
                alert('Failed to unlink gallery')
            }
        } catch (error) {
            console.error('Error unlinking gallery:', error)
            alert('Error unlinking gallery')
        }
    }

    // Auto-tag handlers
    const handleAutoTag = async () => {
        setAutoTagging(true)
        setShowAutoTagModal(true)
        try {
            const response = await fetch(`/api/people/${id}/auto-tag?minConfidence=0.6`)
            const data = await response.json()
            setAutoTagSuggestions(data.suggestions || [])
        } catch (error) {
            console.error('Auto-tag error:', error)
            alert('Failed to get auto-tag suggestions')
        } finally {
            setAutoTagging(false)
        }
    }

    const handleToggleSuggestion = (index) => {
        const newSelected = new Set(selectedSuggestions)
        if (newSelected.has(index)) {
            newSelected.delete(index)
        } else {
            newSelected.add(index)
        }
        setSelectedSuggestions(newSelected)
    }

    const handleApplySuggestions = async () => {
        const suggestions = Array.from(selectedSuggestions).map(index => autoTagSuggestions[index])

        try {
            const response = await fetch(`/api/people/${id}/auto-tag/apply`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ suggestions })
            })

            if (response.ok) {
                const result = await response.json()
                setShowAutoTagModal(false)
                setSelectedSuggestions(new Set())
                fetchPerson()
                alert(`Tagged ${result.galleries_tagged} galleries and ${result.videos_tagged} videos!`)
            } else {
                alert('Failed to apply tags')
            }
        } catch (error) {
            console.error('Apply suggestions error:', error)
            alert('Error applying tags')
        }
    }

    const fetchExclusions = async () => {
        try {
            const response = await fetch(`/api/people/${id}/exclusions`)
            const data = await response.json()
            setExclusions(data.exclusions || [])
        } catch (error) {
            console.error('Fetch exclusions error:', error)
        }
    }

    const handleRemoveExclusion = async (exclusionId) => {
        try {
            const response = await fetch(`/api/people/${id}/exclusions/${exclusionId}`, {
                method: 'DELETE'
            })

            if (response.ok) {
                fetchExclusions()
            }
        } catch (error) {
            console.error('Remove exclusion error:', error)
        }
    }

    // Fetch exclusions when person loads
    useEffect(() => {
        if (person) {
            fetchExclusions()
        }
    }, [person])

    if (loading) {
        return <div className="loading">Loading...</div>
    }

    if (error) {
        return <div className="error">Error: {error}</div>
    }

    if (!person) {
        return <div className="error">Person not found</div>
    }

    const galleries = person.galleries || []

    // enhance galleries with separate lists for videos and existing galleries
    const videoGalleries = []
    const photoGalleries = []

    galleries.forEach(g => {
        if (g.images && g.images.length > 0 && g.images[0].type === 'video') {
            videoGalleries.push(g)
        } else {
            photoGalleries.push(g)
        }
    })

    return (
        <div className="person-detail">
            <div className="person-detail-header">
                <button onClick={() => navigate('/people')} className="back-btn">
                    ← Back to People
                </button>

                {/* Clean Title Header */}
                <div className="person-title-header">
                    <h1>{person.name}</h1>
                    {parseAliases(person.aliases).length > 0 && (
                        <div className="aliases-inline">
                            {parseAliases(person.aliases).map((alias, i) => (
                                <span key={i} className="alias-tag">{alias}</span>
                            ))}
                        </div>
                    )}
                </div>

                {/* Identifiers below title */}
                {person.identifiers && person.identifiers.length > 0 && (
                    <div className="identifiers-section">
                        <div className="identifiers-list">
                            {person.identifiers.map(identifier => (
                                <div key={identifier.id} className={`identifier-badge ${identifier.source}`}>
                                    <span className="identifier-source">{identifier.source}</span>
                                    <span className="identifier-id">{identifier.external_id}</span>
                                    <button onClick={() => handleUnlinkIdentifier(identifier.id)} className="remove-identifier" title="Unlink identifier">✕</button>
                                </div>
                            ))}
                        </div>
                    </div>
                )}

                {/* Floating Action Button */}
                <button
                    className="fab-button"
                    onClick={() => setShowAutoTagModal(!showAutoTagModal)}
                    title="Person Actions"
                >
                    ⋮
                </button>

                {/* Slide-out Sidebar */}
                <div className={`admin-sidebar ${showAutoTagModal ? 'open' : ''}`}>
                    <div className="sidebar-header">
                        <h3>Actions</h3>
                        <button className="close-sidebar" onClick={() => setShowAutoTagModal(false)}>✕</button>
                    </div>
                    <div className="sidebar-content">
                        {isEditing ? (
                            <div className="edit-form">
                                <div className="form-group">
                                    <label>Name</label>
                                    <input
                                        type="text"
                                        value={editForm.name}
                                        onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
                                    />
                                </div>
                                <div className="form-group">
                                    <label>Aliases (comma-separated)</label>
                                    <input
                                        type="text"
                                        value={editForm.aliases}
                                        onChange={(e) => setEditForm({ ...editForm, aliases: e.target.value })}
                                        placeholder="alias1, alias2, alias3"
                                    />
                                </div>
                                <div className="edit-actions">
                                    <button onClick={handleSaveEdit} className="save-btn">Save</button>
                                    <button onClick={() => setIsEditing(false)} className="cancel-btn">Cancel</button>
                                </div>
                            </div>
                        ) : (
                            <>
                                <button onClick={() => setIsEditing(true)} className="sidebar-action-btn">
                                    <div className="btn-text">
                                        <strong>Edit Person</strong>
                                        <small>Update name and aliases</small>
                                    </div>
                                </button>
                                <button onClick={() => {
                                    setIdentifierSearch(person.name)
                                    setShowIdentifierModal(true)
                                }} className="sidebar-action-btn">
                                    <div className="btn-text">
                                        <strong>Identify Person</strong>
                                        <small>Link to StashDB, Babepedia, etc.</small>
                                    </div>
                                </button>
                                <button
                                    onClick={handleAutoTag}
                                    className="sidebar-action-btn"
                                    disabled={!person.identifiers || person.identifiers.length === 0}
                                >
                                    <div className="btn-text">
                                        <strong>Auto-Tag</strong>
                                        <small>{!person.identifiers || person.identifiers.length === 0 ? "Link identifiers first" : "Auto-tag galleries and videos"}</small>
                                    </div>
                                </button>
                            </>
                        )}
                    </div>
                </div>

                {/* Overlay */}
                {showAutoTagModal && <div className="sidebar-overlay" onClick={() => setShowAutoTagModal(false)} />}
            </div>

            {/* Photo Carousel */}
            {person.photos && (() => {
                const photos = JSON.parse(person.photos);
                if (photos.length === 0) return null;

                return (
                    <div className="person-photos-section">
                        <div className="person-photos-carousel">
                            <img
                                src={photos[currentPhotoIndex]}
                                alt={`${person.name} ${currentPhotoIndex + 1}`}
                            />
                            {photos.length > 1 && (
                                <>
                                    <button
                                        className="carousel-btn prev"
                                        onClick={() => setCurrentPhotoIndex((currentPhotoIndex - 1 + photos.length) % photos.length)}
                                    >
                                        ‹
                                    </button>
                                    <button
                                        className="carousel-btn next"
                                        onClick={() => setCurrentPhotoIndex((currentPhotoIndex + 1) % photos.length)}
                                    >
                                        ›
                                    </button>
                                    <div className="carousel-indicators">
                                        {photos.map((_, index) => (
                                            <button
                                                key={index}
                                                className={`indicator ${index === currentPhotoIndex ? 'active' : ''}`}
                                                onClick={() => setCurrentPhotoIndex(index)}
                                            />
                                        ))}
                                    </div>
                                </>
                            )}
                        </div>
                    </div>
                );
            })()}

            {/* Extended Info */}
            {(person.birthdate || person.country || person.ethnicity || person.height || person.measurements || person.hair_color || person.eye_color) && (
                <div className="person-extended-info">
                    <div className="info-grid">
                        {person.birthdate && <div className="info-item"><strong>Born:</strong> {person.birthdate}</div>}
                        {person.country && <div className="info-item"><strong>Country:</strong> {person.country}</div>}
                        {person.ethnicity && <div className="info-item"><strong>Ethnicity:</strong> {person.ethnicity}</div>}
                        {person.height && <div className="info-item"><strong>Height:</strong> {person.height}cm</div>}
                        {person.hair_color && <div className="info-item"><strong>Hair:</strong> {person.hair_color}</div>}
                        {person.eye_color && <div className="info-item"><strong>Eyes:</strong> {person.eye_color}</div>}
                        {person.measurements && <div className="info-item"><strong>Measurements:</strong> {person.measurements}</div>}
                    </div>

                    {(person.tattoos || person.piercings) && (
                        <div className="body-mods-section">
                            <h3>Body Modifications</h3>
                            {person.tattoos && <div className="info-item"><strong>Tattoos:</strong> {person.tattoos}</div>}
                            {person.piercings && <div className="info-item"><strong>Piercings:</strong> {person.piercings}</div>}
                        </div>
                    )}

                    {(person.twitter || person.instagram) && (
                        <div className="social-section">
                            <h3>Social Media</h3>
                            <div className="social-links">
                                {person.twitter && <a href={`https://twitter.com/${person.twitter}`} target="_blank" rel="noopener noreferrer">🐦 Twitter</a>}
                                {person.instagram && <a href={`https://instagram.com/${person.instagram}`} target="_blank" rel="noopener noreferrer">📷 Instagram</a>}
                            </div>
                        </div>
                    )}
                </div>
            )}

            {showIdentifierModal && (
                <div className="modal-overlay" onClick={() => setShowIdentifierModal(false)}>
                    <div className="modal-content" onClick={e => e.stopPropagation()}>
                        <h2>Identify Person</h2>

                        {/* Source selector */}
                        <div className="source-selector">
                            <label>Source:</label>
                            <select value={selectedSource} onChange={(e) => setSelectedSource(e.target.value)}>
                                {availableSources.map(source => (
                                    <option key={source} value={source}>{source.toUpperCase()}</option>
                                ))}
                            </select>
                        </div>

                        <div className="search-box">
                            <input
                                type="text"
                                value={identifierSearch}
                                onChange={(e) => setIdentifierSearch(e.target.value)}
                                placeholder="Search performer name..."
                                onKeyDown={(e) => e.key === 'Enter' && handleSearchIdentifier()}
                            />
                            <button onClick={handleSearchIdentifier} disabled={searchingIdentifier}>
                                {searchingIdentifier ? 'Searching...' : 'Search'}
                            </button>
                        </div>

                        <div className="search-results">
                            {identifierResults.map(result => (
                                <div key={result.external_id} className="search-result-item">
                                    <div className="performer-image">
                                        {result.preview_data?.image_url ? (
                                            <img src={result.preview_data.image_url} alt={result.name} />
                                        ) : (
                                            <div className="no-image">No Image</div>
                                        )}
                                    </div>
                                    <div className="performer-info">
                                        <div className="performer-header">
                                            <strong>{result.name}</strong>
                                            {result.disambiguation && (
                                                <span className="disambiguation">({result.disambiguation})</span>
                                            )}
                                        </div>
                                        <div className="performer-details">
                                            {result.preview_data?.birthdate && <span>Born: {result.preview_data.birthdate}</span>}
                                            {result.preview_data?.country && <span>Country: {result.preview_data.country}</span>}
                                        </div>
                                    </div>
                                    <button onClick={() => handleLinkIdentifier(result.external_id)} className="link-btn">Link</button>
                                </div>
                            ))}
                            {identifierResults.length === 0 && !searchingIdentifier && (
                                <p className="no-results">No results found</p>
                            )}
                        </div>

                        <button className="close-modal-btn" onClick={() => setShowIdentifierModal(false)}>Close</button>
                    </div>
                </div>
            )
            }

            {/* Videos Section */}
            {
                videoGalleries.length > 0 && (
                    <div className="person-galleries">
                        <h2>Videos ({videoGalleries.length})</h2>
                        <div className="galleries-grid">
                            {videoGalleries.map((gallery) => (
                                <div key={gallery.id} className="gallery-card">
                                    <div className="gallery-content" onClick={() => navigate(`/galleries/${gallery.id}`)}>
                                        <div className="gallery-thumbnail video-thumbnail">
                                            {gallery.images && gallery.images.length > 0 ? (
                                                <div className="video-thumb-container">
                                                    <img
                                                        src={`/api/${gallery.images[0].thumbnail_path}`}
                                                        alt={gallery.name}
                                                        loading="lazy"
                                                        onError={(e) => {
                                                            e.target.onerror = null;
                                                            e.target.src = "https://via.placeholder.com/200x200?text=No+Thumb"
                                                        }}
                                                    />
                                                    <div className="play-icon-overlay">▶</div>
                                                </div>
                                            ) : (
                                                <div className="no-image">No Video</div>
                                            )}
                                        </div>
                                        <div className="gallery-info">
                                            <h3>{gallery.name}</h3>
                                        </div>
                                    </div>
                                    <button
                                        onClick={(e) => handleUnlinkGallery(gallery.id, e)}
                                        className="unlink-btn"
                                        title="Unlink video"
                                    >
                                        ✕
                                    </button>
                                </div>
                            ))}
                        </div>
                    </div>
                )
            }

            <div className="person-galleries">
                <h2>Galleries ({photoGalleries.length})</h2>
                {photoGalleries.length === 0 ? (
                    <div className="no-galleries">
                        <p>No photo galleries linked to this person.</p>
                        {videoGalleries.length === 0 && (
                            <p>Click "Link" on the People page to automatically find matching content.</p>
                        )}
                    </div>
                ) : (
                    <div className="galleries-grid">
                        {photoGalleries.map((gallery) => (
                            <div key={gallery.id} className="gallery-card">
                                <div className="gallery-content" onClick={() => navigate(`/galleries/${gallery.id}`)}>
                                    {gallery.images && gallery.images.length > 0 && (
                                        <div className="gallery-thumbnail">
                                            <img
                                                src={`/api/${gallery.images[0].thumbnail_path}`}
                                                alt={gallery.name}
                                                loading="lazy"
                                            />
                                        </div>
                                    )}
                                    <div className="gallery-info">
                                        <h3>{gallery.name}</h3>
                                        <p className="image-count">
                                            {gallery.image_count} {gallery.image_count === 1 ? 'image' : 'images'}
                                        </p>
                                    </div>
                                </div>
                                <button
                                    onClick={(e) => handleUnlinkGallery(gallery.id, e)}
                                    className="unlink-btn"
                                    title="Unlink gallery"
                                >
                                    ✕
                                </button>
                            </div>
                        ))}
                    </div>
                )}
            </div>

            {/* Auto-Tag Modal */}
            <AutoTagModal
                show={showAutoTagModal}
                onClose={() => setShowAutoTagModal(false)}
                suggestions={autoTagSuggestions}
                loading={autoTagging}
                selectedSuggestions={selectedSuggestions}
                onToggleSuggestion={handleToggleSuggestion}
                onApply={handleApplySuggestions}
            />

            {/* Exclusions Section */}
            {
                exclusions.length > 0 && (
                    <div className="exclusions-section">
                        <h2>Excluded Content</h2>
                        <p className="exclusion-help">These galleries and videos have been explicitly marked as NOT featuring this person.</p>
                        <div className="exclusions-list">
                            {exclusions.map(exclusion => (
                                <div key={exclusion.id} className="exclusion-item">
                                    <div className="exclusion-info">
                                        <span className="exclusion-name">{exclusion.gallery ? exclusion.gallery.name : 'Unknown Gallery'}</span>
                                        <span className="exclusion-reason">ID: {exclusion.gallery_id}</span>
                                    </div>
                                    <button
                                        onClick={() => handleRemoveExclusion(exclusion.id)}
                                        className="remove-exclusion-btn"
                                        title="Remove exclusion (allow tagging again)"
                                    >
                                        Restore
                                    </button>
                                </div>
                            ))}
                        </div>
                    </div>
                )
            }
        </div >
    )
}

export default PersonDetail
