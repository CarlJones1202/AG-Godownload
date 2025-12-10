import React from 'react'
import './AutoTag.css'

function AutoTagModal({
    show,
    onClose,
    suggestions,
    loading,
    selectedSuggestions,
    onToggleSuggestion,
    onApply
}) {
    if (!show) return null

    const getConfidenceClass = (confidence) => {
        if (confidence >= 0.8) return 'confidence-high'
        if (confidence >= 0.6) return 'confidence-medium'
        return 'confidence-low'
    }

    const getConfidenceLabel = (confidence) => {
        if (confidence >= 0.8) return 'High'
        if (confidence >= 0.6) return 'Medium'
        return 'Low'
    }

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal-content auto-tag-modal" onClick={e => e.stopPropagation()}>
                <h2>🏷️ Auto-Tag Suggestions</h2>

                {loading ? (
                    <p>Scanning for matches...</p>
                ) : suggestions.length === 0 ? (
                    <p>No matches found. Try linking identifiers to get more aliases!</p>
                ) : (
                    <>
                        <p>Select items to tag:</p>
                        <div className="suggestions-list">
                            {suggestions.map((suggestion, index) => (
                                <div key={index} className="suggestion-item">
                                    <input
                                        type="checkbox"
                                        className="suggestion-checkbox"
                                        checked={selectedSuggestions.has(index)}
                                        onChange={() => onToggleSuggestion(index)}
                                    />
                                    <div className="suggestion-info">
                                        <div className="suggestion-name">{suggestion.name}</div>
                                        <div className="suggestion-meta">
                                            <span className="suggestion-type">📁 {suggestion.type}</span>
                                            <span>Matched: "{suggestion.matched_on}"</span>
                                            <span className={`confidence-badge ${getConfidenceClass(suggestion.confidence)}`}>
                                                {getConfidenceLabel(suggestion.confidence)} ({(suggestion.confidence * 100).toFixed(0)}%)
                                            </span>
                                        </div>
                                    </div>
                                </div>
                            ))}
                        </div>

                        <button
                            className="apply-suggestions-btn"
                            onClick={onApply}
                            disabled={selectedSuggestions.size === 0}
                        >
                            Apply {selectedSuggestions.size} Selected Tag{selectedSuggestions.size !== 1 ? 's' : ''}
                        </button>
                    </>
                )}

                <button className="close-modal-btn" onClick={onClose}>Close</button>
            </div>
        </div>
    )
}

export default AutoTagModal
