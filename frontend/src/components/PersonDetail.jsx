import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import './PersonDetail.css'
import GalleryList from './GalleryList'

function PersonDetail() {
    const { id } = useParams()
    const navigate = useNavigate()
    const [person, setPerson] = useState(null)
    const [loading, setLoading] = useState(true)
    const [isEditing, setIsEditing] = useState(false)
    const [editForm, setEditForm] = useState({ name: '', aliases: '' })

    useEffect(() => {
        fetchPerson()
    }, [id])

    const fetchPerson = async () => {
        setLoading(true)
        try {
            const response = await fetch(`/api/people/${id}`)
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
        } finally {
            setLoading(false)
        }
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
                                <button onClick={() => setIsEditing(true)} className="edit-btn">✏️ Edit</button>
                            </div>
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
                                                src={`/api/thumbnails/${gallery.images[0].filename}`}
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
