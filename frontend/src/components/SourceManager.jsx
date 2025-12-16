import { useState, useEffect } from 'react'
import './SourceManager.css'

function SourceManager({ sources, onSourceAdded, onRefresh, meta, onPageChange, onSearch, searchQuery }) {
    const [showForm, setShowForm] = useState(false)
    const [formData, setFormData] = useState({
        name: '',
        type: 'url',
        location: ''
    })
    const [submitting, setSubmitting] = useState(false)
    const [searchTerm, setSearchTerm] = useState(searchQuery || '')


    // Sync local state if prop changes (e.g. user navigates back/forward)
    useEffect(() => {
        setSearchTerm(searchQuery || '')
    }, [searchQuery])

    const handleSubmit = async (e) => {
        e.preventDefault()
        setSubmitting(true)
        try {
            const response = await fetch('/api/sources', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(formData),
            })
            if (response.ok) {
                setFormData({ name: '', type: 'url', location: '' })
                setShowForm(false)
                onSourceAdded()
            } else {
                const data = await response.json()
                alert(data.error || 'Failed to add source')
            }
        } catch (error) {
            console.error('Error adding source:', error)
            alert('Failed to add source')
        } finally {
            setSubmitting(false)
        }
    }

    const handleCrawl = async (sourceId) => {
        try {
            await fetch(`/api/sources/${sourceId}/crawl`, { method: 'POST' })
            alert('Crawl started! Check back in a few moments.')
            onRefresh()
        } catch (error) {
            console.error('Error starting crawl:', error)
            alert('Failed to start crawl')
        }
    }

    const handleDeleteSource = async (sourceId, e) => {
        e.stopPropagation()
        const deleteGallery = confirm(
            'Delete associated gallery too?\n\nOK = Delete source AND gallery\nCancel = Delete source only'
        )
        let deleteImages = false
        if (deleteGallery) {
            deleteImages = confirm(
                'Delete all images in the gallery?\n\nOK = Delete gallery AND images\nCancel = Delete gallery only'
            )
        }
        if (!confirm(`Are you sure you want to delete this source${deleteGallery ? ' and gallery' : ''}${deleteImages ? ' and images' : ''}?`)) {
            return
        }
        try {
            const params = new URLSearchParams()
            if (deleteGallery) params.append('delete_gallery', 'true')
            if (deleteImages) params.append('delete_images', 'true')
            const url = `/api/sources/${sourceId}${params.toString() ? '?' + params.toString() : ''}`
            const response = await fetch(url, { method: 'DELETE' })
            if (response.ok) {
                onRefresh()
            } else {
                alert('Failed to delete source')
            }
        } catch (error) {
            console.error('Error deleting source:', error)
            alert('Error deleting source')
        }
    }

    useEffect(() => {
        const normalizedSearchQuery = searchQuery || ''
        // Only trigger search if the term has actually changed from what the URL/prop says
        if (searchTerm === normalizedSearchQuery) {
            return
        }

        const delayDebounceFn = setTimeout(() => {
            onSearch(searchTerm)
        }, 500)
        return () => clearTimeout(delayDebounceFn)
    }, [searchTerm, searchQuery])

    // Simple Icons
    const RefreshIcon = () => (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M23 4v6h-6"></path>
            <path d="M1 20v-6h6"></path>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path>
        </svg>
    )

    const TrashIcon = () => (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="3 6 5 6 21 6"></polyline>
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
        </svg>
    )

    const PlusIcon = () => (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <line x1="12" y1="5" x2="12" y2="19"></line>
            <line x1="5" y1="12" x2="19" y2="12"></line>
        </svg>
    )

    const PlayIcon = () => (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polygon points="5 3 19 12 5 21 5 3"></polygon>
        </svg>
    )

    return (
        <div className="source-manager">
            <div className="source-header">
                <h2>Source Manager</h2>
                <div className="source-controls">
                    <div className="search-wrapper">
                        <input
                            type="text"
                            placeholder="Search sources..."
                            className="search-input"
                            value={searchTerm}
                            onChange={(e) => setSearchTerm(e.target.value)}
                        />
                    </div>
                    <button onClick={onRefresh} className="control-btn secondary" title="Refresh">
                        <RefreshIcon />
                    </button>
                    <button onClick={() => setShowForm(!showForm)} className={`control-btn ${showForm ? 'secondary' : 'primary'}`}>
                        {showForm ? 'Cancel' : <><PlusIcon /> Add Source</>}
                    </button>
                </div>
            </div>

            {showForm && (
                <form className="source-form panel" onSubmit={handleSubmit}>
                    <h3>Add New Source</h3>
                    <div className="form-group">
                        <label>Name</label>
                        <input
                            type="text"
                            value={formData.name}
                            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                            required
                            placeholder="e.g. My Favorite Gallery"
                        />
                    </div>
                    <div className="form-group">
                        <label>Type</label>
                        <select
                            value={formData.type}
                            onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                        >
                            <option value="url">URL Scraper</option>
                        </select>
                    </div>
                    <div className="form-group">
                        <label>Location (URL)</label>
                        <input
                            type="url"
                            value={formData.location}
                            onChange={(e) => setFormData({ ...formData, location: e.target.value })}
                            required
                            placeholder="https://example.com/thread/..."
                        />
                    </div>
                    <button type="submit" disabled={submitting} className="submit-btn">
                        {submitting ? 'Adding...' : 'Add Source'}
                    </button>
                </form>
            )}

            <div className="source-list">
                {sources.length === 0 ? (
                    <div className="empty-state">
                        <p>No sources found. Add one to start collecting content!</p>
                    </div>
                ) : (
                    sources.map(source => (
                        <div key={source.id} className="source-item">
                            <div className="source-main">
                                <div className="source-info-header">
                                    <h3>{source.name}</h3>
                                    <span className={`status-badge ${source.status || 'idle'}`}>
                                        {source.status || 'idle'}
                                    </span>
                                </div>
                                <a href={source.location} target="_blank" rel="noopener noreferrer" className="source-url">
                                    {source.location}
                                </a>
                                <div className="source-meta">
                                    {source.last_checked_at && source.last_checked_at !== '0001-01-01T00:00:00Z' ? (
                                        <span>Last checked: {new Date(source.last_checked_at).toLocaleString()}</span>
                                    ) : (
                                        <span>Never checked</span>
                                    )}
                                </div>
                            </div>
                            <div className="source-actions">
                                <button
                                    onClick={() => handleCrawl(source.id)}
                                    disabled={source.status === 'crawling'}
                                    className="action-btn secondary"
                                    title="Start Crawl"
                                >
                                    <PlayIcon /> {source.status === 'crawling' ? 'Running...' : 'Run'}
                                </button>
                                <button
                                    className="action-btn danger-ghost"
                                    onClick={(e) => handleDeleteSource(source.id, e)}
                                    title="Delete Source"
                                >
                                    <TrashIcon />
                                </button>
                            </div>
                        </div>
                    ))
                )}
            </div>

            {meta && meta.total_pages > 1 && (
                <div className="pagination">
                    <button
                        disabled={meta.current_page === 1}
                        onClick={() => onPageChange(meta.current_page - 1)}
                        className="page-btn"
                    >
                        Previous
                    </button>
                    <span>Page {meta.current_page} of {meta.total_pages}</span>
                    <button
                        disabled={meta.current_page === meta.total_pages}
                        onClick={() => onPageChange(meta.current_page + 1)}
                        className="page-btn"
                    >
                        Next
                    </button>
                </div>
            )}
        </div>
    )
}

export default SourceManager
