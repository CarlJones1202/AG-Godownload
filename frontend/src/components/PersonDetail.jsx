import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import './PersonDetail.css'
import './PersonDetail_identifiers.css'
import './AutoTag.css'
import PersonStats from './PersonStats'
import AutoTagModal from './AutoTagModal'
import SourceScanModal from './SourceScanModal'
import GalleryCard from './GalleryCard'
import SortControls from './SortControls'

function PersonDetail() {
    const { id } = useParams()
    const navigate = useNavigate()
    const [searchParams, setSearchParams] = useSearchParams()
    const [person, setPerson] = useState(null)
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState(null)
    const [isEditing, setIsEditing] = useState(false)
    const [editForm, setEditForm] = useState({ name: '', aliases: '' })
    const [currentPhotoIndex, setCurrentPhotoIndex] = useState(0)

    const sort = searchParams.get('sort') || 'newest'
    const seed = searchParams.get('seed') || '0'

    const fetchPerson = useCallback(async () => {
        setLoading(true)
        setError(null)
        try {
            const response = await fetch(`/api/people/${id}?sort=${sort}&seed=${seed}`)
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
    }, [id, sort, seed])

    useEffect(() => {
        fetchPerson()
    }, [fetchPerson])

    const handleSortChange = (newSort) => {
        setSearchParams(prev => {
            prev.set('sort', newSort)
            if (newSort !== 'shuffle') {
                prev.delete('seed')
            } else if (!prev.get('seed')) {
                prev.set('seed', Math.floor(Math.random() * 1000000).toString())
            }
            return prev
        })
    }

    const handleSeedChange = (newSeed) => {
        setSearchParams(prev => {
            prev.set('seed', newSeed.toString())
            return prev
        })
    }

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

    // Source scan state
    const [showSourceScanModal, setShowSourceScanModal] = useState(false)

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
            <a href="/people" className="back-btn" onClick={(e) => {
                e.preventDefault();
                navigate('/people');
            }}>
                ← Back to People
            </a>

            <div className="person-profile-container">
                {/* Left Sidebar: Photo & Social */}
                <div className="person-sidebar">
                    {(() => {
                        let photos = [];
                        try {
                            if (person.photos) {
                                photos = JSON.parse(person.photos);
                            }
                        } catch (e) {
                            console.error('Failed to parse photos:', e);
                        }

                        // Use actual photos if they exist
                        if (photos && Array.isArray(photos) && photos.length > 0) {
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
                        }

                        // Fallback to the thumbnail_path from backend
                        if (person.thumbnail_path) {
                            return (
                                <div className="person-photos-section">
                                    <div className="person-fallback-thumbnail">
                                        <img src={person.thumbnail_path} alt={person.name} />
                                    </div>
                                </div>
                            );
                        }

                        // Absolute fallback to initial
                        return <div className="no-photo-placeholder">{person.name[0]}</div>;
                    })()}

                    {(person.twitter || person.instagram) && (
                        <div className="social-section-sidebar">
                            <div className="social-links">
                                {person.twitter && <a href={`https://twitter.com/${person.twitter}`} target="_blank" rel="noopener noreferrer">Twitter</a>}
                                {person.instagram && <a href={`https://instagram.com/${person.instagram}`} target="_blank" rel="noopener noreferrer">Instagram</a>}
                            </div>
                        </div>
                    )}
                </div>

                {/* Main Content: Info & Details */}
                <div className="person-main-content">
                    <div className="person-header-row">
                        <div className="title-section">
                            <h1>{person.name}</h1>
                            {parseAliases(person.aliases).length > 0 && (
                                <div className="aliases-row">
                                    {parseAliases(person.aliases).map((alias, i) => (
                                        <span key={i} className="alias-tag">{alias}</span>
                                    ))}
                                </div>
                            )}
                        </div>
                        <div className="header-actions">
                            <button onClick={() => setIsEditing(!isEditing)} className="action-btn secondary">
                                {isEditing ? 'Cancel' : 'Edit'}
                            </button>
                            <button onClick={() => {
                                setIdentifierSearch(person.name)
                                setShowIdentifierModal(true)
                            }} className="action-btn secondary">
                                Identify
                            </button>
                            <button
                                onClick={handleAutoTag}
                                className="action-btn primary"
                                disabled={!person.identifiers || person.identifiers.length === 0}
                            >
                                Auto-Tag
                            </button>
                            <button
                                onClick={() => setShowSourceScanModal(true)}
                                className="action-btn secondary"
                            >
                                Scan Sources
                            </button>
                        </div>
                    </div>

                    {/* Identifiers Row */}
                    {person.identifiers && person.identifiers.length > 0 && (
                        <div className="identifiers-row">
                            {person.identifiers.map(identifier => (
                                <div key={identifier.ID} className={`identifier-badge ${identifier.Source}`}>
                                    <span className="identifier-source">{identifier.Source}</span>
                                    <a
                                        href={identifier.WaitURL || identifier.Url || '#'}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className="identifier-id"
                                    >
                                        {identifier.ExternalID}
                                    </a>
                                    <button
                                        className="remove-identifier"
                                        onClick={() => handleUnlinkIdentifier(identifier.ID)}
                                        title="Unlink identifier"
                                    >
                                        ✕
                                    </button>
                                </div>
                            ))}
                        </div>
                    )}

                    {isEditing ? (
                        <div className="edit-form-panel">
                            <div className="form-group">
                                <label>Name</label>
                                <input
                                    type="text"
                                    value={editForm.name}
                                    onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
                                />
                            </div>
                            <div className="form-group">
                                <label>Aliases</label>
                                <input
                                    type="text"
                                    value={editForm.aliases}
                                    onChange={(e) => setEditForm({ ...editForm, aliases: e.target.value })}
                                    placeholder="comma, separated, aliases"
                                />
                            </div>
                            <button onClick={handleSaveEdit} className="save-btn">Save Changes</button>
                        </div>
                    ) : (
                        <div className="info-grid-compact">
                            {person.birthdate && <div className="info-item"><span className="label">Born</span> <span className="value">{person.birthdate}</span></div>}
                            {person.country && <div className="info-item"><span className="label">Country</span> <span className="value">{person.country}</span></div>}
                            {person.ethnicity && <div className="info-item"><span className="label">Ethnicity</span> <span className="value">{person.ethnicity}</span></div>}
                            {person.height && <div className="info-item"><span className="label">Height</span> <span className="value">{person.height}cm</span></div>}
                            {person.hair_color && <div className="info-item"><span className="label">Hair</span> <span className="value">{person.hair_color}</span></div>}
                            {person.eye_color && <div className="info-item"><span className="label">Eyes</span> <span className="value">{person.eye_color}</span></div>}
                            {person.measurements && <div className="info-item"><span className="label">Measurements</span> <span className="value">{person.measurements}</span></div>}
                            {person.tattoos && <div className="info-item wide"><span className="label">Tattoos</span> <span className="value">{person.tattoos}</span></div>}
                            {person.piercings && <div className="info-item wide"><span className="label">Piercings</span> <span className="value">{person.piercings}</span></div>}
                        </div>
                    )}

                    {/* Stats Section */}
                    {!isEditing && <PersonStats personId={person.id} />}
                </div>
            </div>

            {/* Videos Section */}
            {videoGalleries.length > 0 && (
                <div className="content-section">
                    <h2>Videos <span className="count-badge">{videoGalleries.length}</span></h2>
                    <div className="galleries-grid">
                        {videoGalleries.map((gallery) => (
                            <GalleryCard
                                key={gallery.id}
                                gallery={gallery}
                                onClick={() => navigate(`/galleries/${gallery.id}`)}
                                actionNode={
                                    <button
                                        onClick={(e) => handleUnlinkGallery(gallery.id, e)}
                                        className="unlink-btn"
                                        title="Unlink video"
                                    >
                                        ✕
                                    </button>
                                }
                            />
                        ))}
                    </div>
                </div>
            )}

            {/* Galleries Section */}
            <div className="content-section">
                <div className="section-header-flex">
                    <h2>Galleries <span className="count-badge">{photoGalleries.length}</span></h2>
                    <SortControls
                        sort={sort}
                        setSort={handleSortChange}
                        seed={parseInt(seed)}
                        setSeed={handleSeedChange}
                        onRandomize={handleSeedChange}
                    />
                </div>
                {photoGalleries.length === 0 && videoGalleries.length === 0 ? (
                    <div className="empty-state">
                        <p>No content linked to this person.</p>
                    </div>
                ) : (
                    <div className="galleries-grid">
                        {photoGalleries.map((gallery) => (
                            <GalleryCard
                                key={gallery.id}
                                gallery={gallery}
                                onClick={() => navigate(`/galleries/${gallery.id}`)}
                                actionNode={
                                    <button
                                        onClick={(e) => handleUnlinkGallery(gallery.id, e)}
                                        className="unlink-btn"
                                        title="Unlink gallery"
                                    >
                                        ✕
                                    </button>
                                }
                            />
                        ))}
                    </div>
                )}
            </div>

            <AutoTagModal
                show={showAutoTagModal}
                onClose={() => setShowAutoTagModal(false)}
                suggestions={autoTagSuggestions}
                loading={autoTagging}
                selectedSuggestions={selectedSuggestions}
                onToggleSuggestion={handleToggleSuggestion}
                onApply={handleApplySuggestions}
            />

            <SourceScanModal
                show={showSourceScanModal}
                onClose={() => setShowSourceScanModal(false)}
                personId={person?.id}
                personName={person?.name}
                personAliases={person?.aliases}
            />

            {showIdentifierModal && (
                <div className="modal-overlay" onClick={() => setShowIdentifierModal(false)}>
                    <div className="modal-content identify-modal" onClick={e => e.stopPropagation()}>
                        <h2>Identify Person</h2>
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
                                        ) : <div className="no-image">No Image</div>}
                                    </div>
                                    <div className="performer-info">
                                        <div className="performer-header">
                                            <strong>{result.name}</strong>
                                            {result.disambiguation && <span className="disambiguation">({result.disambiguation})</span>}
                                        </div>
                                        <div className="performer-details">
                                            {result.preview_data?.birthdate && <span>Born: {result.preview_data.birthdate}</span>}
                                            {result.preview_data?.country && <span>Country: {result.preview_data.country}</span>}
                                        </div>
                                    </div>
                                    <button onClick={() => handleLinkIdentifier(result.external_id)} className="link-btn">Link</button>
                                </div>
                            ))}
                            {identifierResults.length === 0 && !searchingIdentifier && <p className="no-results">No results found</p>}
                        </div>
                        <button className="close-modal-btn" onClick={() => setShowIdentifierModal(false)}>Close</button>
                    </div>
                </div>
            )}

            {exclusions.length > 0 && (
                <div className="exclusions-section">
                    <h2>Excluded Content</h2>
                    <div className="exclusions-list">
                        {exclusions.map(exclusion => (
                            <div key={exclusion.id} className="exclusion-item">
                                <span>{exclusion.gallery ? exclusion.gallery.name : exclusion.gallery_id}</span>
                                <button onClick={() => handleRemoveExclusion(exclusion.id)}>Restore</button>
                            </div>
                        ))}
                    </div>
                </div>
            )}
        </div>
    )
}

export default PersonDetail
