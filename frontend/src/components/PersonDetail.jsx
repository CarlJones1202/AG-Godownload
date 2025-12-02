import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import './PersonDetail.css'
import GalleryList from './GalleryList'

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

    const [stashSearch, setStashSearch] = useState('')
    const [stashResults, setStashResults] = useState([])
    const [showStashModal, setShowStashModal] = useState(false)
    const [searchingStash, setSearchingStash] = useState(false)

    const handleSearchStash = async () => {
        if (!stashSearch.trim()) return
        setSearchingStash(true)
        try {
            const response = await fetch(`/api/stashdb/search?name=${encodeURIComponent(stashSearch)}`)
            const result = await response.json()
            if (result.data) {
                setStashResults(result.data)
            }
        } catch (error) {
            console.error('Failed to search StashDB:', error)
            alert('Failed to search StashDB')
        } finally {
            setSearchingStash(false)
        }
    }

    const handleLinkStash = async (stashId) => {
        try {
            const response = await fetch(`/api/people/${id}/stashdb/link`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ stash_id: stashId })
            })

            if (response.ok) {
                setShowStashModal(false)
                setStashResults([])
                setStashSearch('')
                fetchPerson()
                alert('Successfully linked to StashDB!')
            } else {
                alert('Failed to link to StashDB')
            }
        } catch (error) {
            console.error('Failed to link StashDB:', error)
            alert('Failed to link StashDB')
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

    return (
        <div className="person-detail">
            <div className="person-detail-header">
                <button onClick={() => navigate('/people')} className="back-btn">
                    ← Back to People
                </button>
                <div className="person-info-header">
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
                            <div className="person-title">
                                <h1>{person.name}</h1>
                                <div className="person-actions">
                                    <button onClick={() => setIsEditing(true)} className="edit-btn">✏️ Edit</button>
                                    <button onClick={() => {
                                        setStashSearch(person.name)
                                        setShowStashModal(true)
                                    }} className="stash-btn">🔗 Link StashDB</button>
                                </div>
                            </div>
                            {person.stash_id && (
                                <div className="stash-badge">
                                    <span className="stash-icon">✓</span> Linked to StashDB
                                </div>
                            )}

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

                            {parseAliases(person.aliases).length > 0 && (
                                <div className="aliases">
                                    {parseAliases(person.aliases).map((alias, i) => (
                                        <span key={i} className="alias-tag">{alias}</span>
                                    ))}
                                </div>
                            )}
                        </>
                    )}
                </div>
            </div>

            {showStashModal && (
                <div className="modal-overlay" onClick={() => setShowStashModal(false)}>
                    <div className="modal-content" onClick={e => e.stopPropagation()}>
                        <h2>Link to StashDB</h2>
                        <div className="search-box">
                            <input
                                type="text"
                                value={stashSearch}
                                onChange={(e) => setStashSearch(e.target.value)}
                                placeholder="Search performer name..."
                                onKeyDown={(e) => e.key === 'Enter' && handleSearchStash()}
                            />
                            <button onClick={handleSearchStash} disabled={searchingStash}>
                                {searchingStash ? 'Searching...' : 'Search'}
                            </button>
                        </div>

                        <div className="search-results">
                            {stashResults.map(performer => (
                                <div key={performer.id} className="search-result-item">
                                    <div className="performer-image">
                                        {performer.images && performer.images.length > 0 ? (
                                            <img src={performer.images[0].url} alt={performer.name} />
                                        ) : (
                                            <div className="no-image">No Image</div>
                                        )}
                                    </div>
                                    <div className="performer-info">
                                        <div className="performer-header">
                                            <strong>{performer.name}</strong>
                                            {performer.disambiguation && (
                                                <span className="disambiguation">({performer.disambiguation})</span>
                                            )}
                                        </div>
                                        <div className="performer-details">
                                            {performer.birthdate && performer.birthdate.date && <span>Born: {performer.birthdate.date}</span>}
                                            {performer.country && <span>Country: {performer.country}</span>}
                                        </div>
                                        {performer.aliases && performer.aliases.length > 0 && (
                                            <div className="performer-aliases">
                                                Aliases: {performer.aliases.join(', ')}
                                            </div>
                                        )}
                                    </div>
                                    <button onClick={() => handleLinkStash(performer.id)} className="link-btn">Link</button>
                                </div>
                            ))}
                            {stashResults.length === 0 && !searchingStash && (
                                <p className="no-results">No results found</p>
                            )}
                        </div>

                        <button className="close-modal-btn" onClick={() => setShowStashModal(false)}>Close</button>
                    </div>
                </div>
            )}

            <div className="person-galleries">
                <h2>Galleries ({galleries.length})</h2>
                {galleries.length === 0 ? (
                    <div className="no-galleries">
                        <p>No galleries linked to this person yet.</p>
                        <p>Click "Link" on the People page to automatically find matching galleries.</p>
                    </div>
                ) : (
                    <div className="galleries-grid">
                        {galleries.map((gallery) => (
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
        </div>
    )
}

export default PersonDetail
