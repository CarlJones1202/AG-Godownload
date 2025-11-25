import { useState } from 'react'
import './SourceManager.css'

function SourceManager({ sources, onSourceAdded, onRefresh }) {
    const [showForm, setShowForm] = useState(false)
    const [formData, setFormData] = useState({
        name: '',
        type: 'url',
        location: ''
    })
    const [submitting, setSubmitting] = useState(false)

    const handleSubmit = async (e) => {
        e.preventDefault()
        setSubmitting(true)

        try {
            const response = await fetch('/api/sources', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(formData),
            })

            if (response.ok) {
                setFormData({ name: '', type: 'url', location: '' })
                setShowForm(false)
                onSourceAdded()
            } else {
                alert('Failed to add source')
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
            await fetch(`/api/sources/${sourceId}/crawl`, {
                method: 'POST',
            })
            alert('Crawl started! Check back in a few moments.')
            onRefresh()
        } catch (error) {
            console.error('Error starting crawl:', error)
            alert('Failed to start crawl')
        }
    }

    return (
        <div className="source-manager">
            <div className="source-header">
                <h2>Sources</h2>
                <div>
                    <button onClick={onRefresh}>🔄 Refresh</button>
                    <button onClick={() => setShowForm(!showForm)}>
                        {showForm ? 'Cancel' : '+ Add Source'}
                    </button>
                </div>
            </div>

            {showForm && (
                <form className="source-form" onSubmit={handleSubmit}>
                    <div className="form-group">
                        <label>Name</label>
                        <input
                            type="text"
                            value={formData.name}
                            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                            required
                            placeholder="My Gallery"
                        />
                    </div>
                    <div className="form-group">
                        <label>Type</label>
                        <select
                            value={formData.type}
                            onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                        >
                            <option value="url">URL</option>
                        </select>
                    </div>
                    <div className="form-group">
                        <label>Location (URL)</label>
                        <input
                            type="url"
                            value={formData.location}
                            onChange={(e) => setFormData({ ...formData, location: e.target.value })}
                            required
                            placeholder="https://example.com/thread/12345"
                        />
                    </div>
                    <button type="submit" disabled={submitting}>
                        {submitting ? 'Adding...' : 'Add Source'}
                    </button>
                </form>
            )}

            <div className="source-list">
                {sources.length === 0 ? (
                    <div className="empty-state">
                        <p>No sources yet. Add one to start crawling!</p>
                    </div>
                ) : (
                    sources.map(source => (
                        <div key={source.id} className="source-card">
                            <div className="source-info">
                                <h3>{source.name}</h3>
                                <p className="source-url">{source.location}</p>
                                <div className="source-meta">
                                    <span className={`status status-${source.status || 'idle'}`}>
                                        {source.status || 'idle'}
                                    </span>
                                    {source.last_checked_at && source.last_checked_at !== '0001-01-01T00:00:00Z' && (
                                        <span className="last-checked">
                                            Last checked: {new Date(source.last_checked_at).toLocaleString()}
                                        </span>
                                    )}
                                </div>
                            </div>
                            <button
                                onClick={() => handleCrawl(source.id)}
                                disabled={source.status === 'crawling'}
                            >
                                {source.status === 'crawling' ? 'Crawling...' : '🔄 Crawl Now'}
                            </button>
                        </div>
                    ))
                )}
            </div>
        </div>
    )
}

export default SourceManager
