import { useState, useEffect } from 'react'
import './PersonPicker.css'

function PersonPicker({ onPersonSelect }) {
    const [searchQuery, setSearchQuery] = useState('')
    const [people, setPeople] = useState([])
    const [loading, setLoading] = useState(false)

    useEffect(() => {
        const fetchPeople = async () => {
            setLoading(true)
            try {
                const query = searchQuery ? `&q=${encodeURIComponent(searchQuery)}` : ''
                const response = await fetch(`/api/people?limit=20${query}`)
                const result = await response.json()
                if (result.data) {
                    setPeople(result.data)
                }
            } catch (error) {
                console.error('Failed to fetch people:', error)
            } finally {
                setLoading(false)
            }
        }

        // Debounce search
        const timeoutId = setTimeout(() => {
            fetchPeople()
        }, 300)

        return () => clearTimeout(timeoutId)
    }, [searchQuery])

    return (
        <div className="person-picker">
            <div className="person-search-container">
                <input
                    type="text"
                    className="person-search-input"
                    placeholder="Search people..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    autoFocus
                />
            </div>

            <div className="person-list">
                {loading ? (
                    <div className="person-loading">Loading...</div>
                ) : people.length > 0 ? (
                    people.map(person => (
                        <button
                            key={person.id}
                            className="person-item"
                            onClick={() => onPersonSelect(person)}
                        >
                            <div className="person-avatar">
                                {person.thumbnail_path ? (
                                    <img src={person.thumbnail_path} alt={person.name} />
                                ) : (
                                    <div className="person-avatar-placeholder">
                                        {person.name.charAt(0).toUpperCase()}
                                    </div>
                                )}
                            </div>
                            <div className="person-info">
                                <span className="person-name">{person.name}</span>
                                <span className="person-count">{person.gallery_count} galleries</span>
                            </div>
                        </button>
                    ))
                ) : (
                    <div className="person-empty">No people found</div>
                )}
            </div>
        </div>
    )
}

export default PersonPicker
