import { useState } from 'react'
import './SearchBar.css'

function SearchBar({ onSearch, onColorFilter, onPersonFilter }) {
    const [searchText, setSearchText] = useState('')
    const [showColorPicker, setShowColorPicker] = useState(false)
    const [showPersonList, setShowPersonList] = useState(false)

    const handleSubmit = (e) => {
        e.preventDefault()
        if (onSearch) {
            onSearch(searchText)
        }
    }

    const handleColorClick = () => {
        setShowColorPicker(!showColorPicker)
        setShowPersonList(false)
    }

    const handlePersonClick = () => {
        setShowPersonList(!showPersonList)
        setShowColorPicker(false)
    }

    return (
        <div className="search-bar-container">
            <form className="search-bar" onSubmit={handleSubmit}>
                <input
                    type="text"
                    className="search-input"
                    placeholder="Search..."
                    value={searchText}
                    onChange={(e) => setSearchText(e.target.value)}
                />
                <div className="search-divider"></div>
                <button
                    type="button"
                    className={`search-icon-btn ${showColorPicker ? 'active' : ''}`}
                    onClick={handleColorClick}
                    title="Filter by color"
                >
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="10"></circle>
                        <path d="M12 2a10 10 0 0 0 0 20"></path>
                        <path d="M12 2a10 10 0 0 1 0 20"></path>
                        <line x1="12" y1="2" x2="12" y2="22"></line>
                        <line x1="2" y1="12" x2="22" y2="12"></line>
                    </svg>
                </button>
                <button
                    type="button"
                    className={`search-icon-btn ${showPersonList ? 'active' : ''}`}
                    onClick={handlePersonClick}
                    title="Filter by person"
                >
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path>
                        <circle cx="12" cy="7" r="4"></circle>
                    </svg>
                </button>
            </form>

            {showColorPicker && (
                <div className="filter-dropdown color-picker-dropdown">
                    <div className="dropdown-header">
                        <h3>Filter by Color</h3>
                        <button className="close-btn" onClick={() => setShowColorPicker(false)}>✕</button>
                    </div>
                    <div className="dropdown-content">
                        <p className="placeholder-text">Color wheel coming soon...</p>
                        {/* Color wheel will be implemented here */}
                    </div>
                </div>
            )}

            {showPersonList && (
                <div className="filter-dropdown person-list-dropdown">
                    <div className="dropdown-header">
                        <h3>Filter by Person</h3>
                        <button className="close-btn" onClick={() => setShowPersonList(false)}>✕</button>
                    </div>
                    <div className="dropdown-content">
                        <p className="placeholder-text">Person list coming soon...</p>
                        {/* Person list will be implemented here */}
                    </div>
                </div>
            )}
        </div>
    )
}

export default SearchBar
