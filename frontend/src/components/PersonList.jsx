import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import './PersonList.css'

function PersonList({ people, onRefresh, meta, onPageChange }) {
    const navigate = useNavigate()
    const [showForm, setShowForm] = useState(false)
    const [editingPerson, setEditingPerson] = useState(null)
    const [formData, setFormData] = useState({ name: '', aliases: '' })

    const handleCreatePerson = async (e) => {
        e.preventDefault()

        const aliases = formData.aliases
            .split(',')
            .map(a => a.trim())
            .filter(a => a.length > 0)

        try {
            const response = await fetch('/api/people', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: formData.name,
                    aliases: aliases
                })
            })

            if (response.ok) {
                setShowForm(false)
                setFormData({ name: '', aliases: '' })
                onRefresh()
            } else {
                alert('Failed to create person')
            }
        } catch (error) {
            console.error('Error creating person:', error)
            alert('Error creating person')
        }
    }

    const handleLinkGalleries = async (personId) => {
        try {
            const response = await fetch(`/api/people/${personId}/link-galleries`, {
                method: 'POST'
            })

            if (response.ok) {
                const result = await response.json()
                alert(`Linked ${result.matched_count} galleries`)
                onRefresh()
            } else {
                alert('Failed to link galleries')
            }
        } catch (error) {
            console.error('Error linking galleries:', error)
            alert('Error linking galleries')
        }
    }

    const handleDeletePerson = async (personId, e) => {
        e.stopPropagation()

        if (!confirm('Are you sure you want to delete this person?')) {
            return
        }

        try {
            const response = await fetch(`/api/people/${personId}`, {
                method: 'DELETE'
            })

            if (response.ok) {
                onRefresh()
            } else {
                alert('Failed to delete person')
            }
        } catch (error) {
            console.error('Error deleting person:', error)
            alert('Error deleting person')
        }
    }

    const parseAliases = (aliasesStr) => {
        try {
            return JSON.parse(aliasesStr || '[]')
        } catch {
            return []
        }
    }

    return (
        <div className="person-list">
            <div className="person-header">
                <h2>People</h2>
                <button onClick={() => setShowForm(!showForm)}>
                    {showForm ? '✕ Cancel' : '+ Add Person'}
                </button>
            </div>

            {showForm && (
                <div className="person-form-card">
                    <form onSubmit={handleCreatePerson}>
                        <div className="form-group">
                            <label>Name</label>
                            <input
                                type="text"
                                value={formData.name}
                                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                                required
                            />
                        </div>
                        <div className="form-group">
                            <label>Aliases (comma-separated)</label>
                            <input
                                type="text"
                                value={formData.aliases}
                                onChange={(e) => setFormData({ ...formData, aliases: e.target.value })}
                                placeholder="alias1, alias2, alias3"
                            />
                        </div>
                        <button type="submit">Create Person</button>
                    </form>
                </div>
            )}

            {people.length === 0 ? (
                <div className="no-people">
                    <p>No people found. Add a person to get started!</p>
                </div>
            ) : (
                <div className="people-grid">
                    {people.map((person) => (
                        <div
                            key={person.id}
                            className="person-card"
                            onClick={() => navigate(`/people/${person.id}`)}
                        >
                            <div className="person-thumbnail">
                                {person.thumbnail_path ? (
                                    <img
                                        src={person.thumbnail_path}
                                        alt={person.name}
                                        loading="lazy"
                                    />
                                ) : (
                                    <div className="no-image">👤</div>
                                )}
                            </div>
                            <div className="person-info">
                                <h3>{person.name}</h3>
                                {(person.country || person.birthdate) && (
                                    <div className="person-meta">
                                        {person.country && <span className="meta-tag">🗺️ {person.country}</span>}
                                        {person.birthdate && <span className="meta-tag">🎂 {person.birthdate}</span>}
                                    </div>
                                )}
                                {parseAliases(person.aliases).length > 0 && (
                                    <div className="aliases">
                                        {parseAliases(person.aliases).map((alias, i) => (
                                            <span key={i} className="alias-tag">{alias}</span>
                                        ))}
                                    </div>
                                )}
                                <p className="gallery-count">
                                    {person.gallery_count || 0} {person.gallery_count === 1 ? 'gallery' : 'galleries'}
                                </p>
                            </div>
                            <div className="person-actions">
                                <button
                                    onClick={() => handleLinkGalleries(person.id)}
                                    className="link-btn"
                                    title="Link to galleries"
                                >
                                    🔗 Link
                                </button>
                                <button
                                    onClick={(e) => handleDeletePerson(person.id, e)}
                                    className="delete-btn"
                                    title="Delete person"
                                >
                                    🗑️
                                </button>
                            </div>
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

export default PersonList
