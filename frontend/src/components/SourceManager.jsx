import { useState, useEffect } from 'react'
import './SourceManager.css'

function SourceManager({ sources, onSourceAdded, onRefresh, meta, onPageChange, onSearch, searchQuery }) {
    const [showForm, setShowForm] = useState(false)
    const [formData, setFormData] = useState({
        name: '',
        type: 'url',
        location: '',
        priority: 0
    })
    const [submitting, setSubmitting] = useState(false)
    const [searchTerm, setSearchTerm] = useState(searchQuery || '')
    const [globalStatus, setGlobalStatus] = useState(null)
    const [showMonitor, setShowMonitor] = useState(true)

    // WebSocket for real-time updates
    useEffect(() => {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
        const host = window.location.host
        const socketUrl = `${protocol}//${host}/api/ws`

        let socket
        let reconnectTimeout

        const connect = () => {
            console.log('Connecting to WebSocket status...')
            socket = new WebSocket(socketUrl)

            socket.onmessage = (event) => {
                try {
                    const status = JSON.parse(event.data)

                    setGlobalStatus(prevStatus => {
                        // Only refresh the main sources list if activity starts or stops
                        // This prevents constant API calls during progress updates
                        const prevActiveIds = prevStatus?.crawler?.active_sources?.map(s => s.id).sort().join(',') || ''
                        const newActiveIds = status.crawler?.active_sources?.map(s => s.id).sort().join(',') || ''

                        const verificationChanged = prevStatus?.verification?.is_running !== status.verification?.is_running
                        const videosChanged = prevStatus?.videos?.is_running !== status.videos?.is_running

                        if (prevActiveIds !== newActiveIds || verificationChanged || videosChanged) {
                            console.log('Activity state changed, refreshing sources list...')
                            onRefresh()
                        }
                        return status
                    })
                } catch (error) {
                    console.error('Error parsing WebSocket message:', error)
                }
            }

            socket.onclose = () => {
                console.log('WebSocket disconnected, reconnecting...')
                reconnectTimeout = setTimeout(connect, 3000)
            }

            socket.onerror = (error) => {
                console.error('WebSocket error:', error)
                socket.close()
            }
        }

        connect()

        return () => {
            if (socket) socket.close()
            if (reconnectTimeout) clearTimeout(reconnectTimeout)
        }
    }, [onRefresh])


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
                setFormData({ name: '', type: 'url', location: '', priority: 0 })
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

    const handlePriorityChange = async (sourceId, newPriority) => {
        try {
            const response = await fetch(`/api/sources/${sourceId}/priority`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ priority: parseInt(newPriority) }),
            })
            if (response.ok) {
                onRefresh()
            }
        } catch (error) {
            console.error('Error updating priority:', error)
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
                    <div className="form-group">
                        <label>Priority</label>
                        <select
                            value={formData.priority}
                            onChange={(e) => setFormData({ ...formData, priority: parseInt(e.target.value) })}
                        >
                            <option value="0">Default (0)</option>
                            <option value="1">Low (1)</option>
                            <option value="2">Medium (2)</option>
                            <option value="3">High (3)</option>
                        </select>
                    </div>
                    <button type="submit" disabled={submitting} className="submit-btn">
                        {submitting ? 'Adding...' : 'Add Source'}
                    </button>
                </form>
            )}

            {globalStatus && (globalStatus.verification?.is_running || globalStatus.videos?.is_running || globalStatus.crawler?.active_sources?.length > 0) && (
                <div className="global-monitor">
                    <div className="monitor-header" onClick={() => setShowMonitor(!showMonitor)}>
                        <h3>
                            <span className="active-indicator"></span>
                            Active Download Activity
                        </h3>
                        <div className="monitor-toggle">{showMonitor ? 'Collapse' : 'Expand'}</div>
                    </div>
                    {showMonitor && (
                        <div className="monitor-content">
                            {globalStatus.verification?.is_running && (
                                <div className="monitor-section">
                                    <h4>Image Verification</h4>
                                    <div className="activity-item">
                                        <div className="activity-info">
                                            <span>Progress</span>
                                            <span>{globalStatus.verification.processed} / {globalStatus.verification.total_images}</span>
                                        </div>
                                        <div className="progress-container">
                                            <div
                                                className="progress-bar"
                                                style={{ width: `${(globalStatus.verification.processed / globalStatus.verification.total_images) * 100}%` }}
                                            ></div>
                                        </div>
                                        <div className="progress-stats">
                                            <span>Found: {globalStatus.verification.missing_found}</span>
                                            <span>Recovered: {globalStatus.verification.recovered}</span>
                                        </div>
                                    </div>
                                    <div className="provider-grid">
                                        {Object.entries(globalStatus.verification.provider_status || {}).map(([name, status]) => (
                                            <div key={name} className="provider-stat">
                                                <span className="provider-name">{name}</span>
                                                <span className="provider-count">{status.active}/{status.max_allowed}</span>
                                            </div>
                                        ))}
                                    </div>
                                    {globalStatus.verification.active_downloads?.length > 0 && (
                                        <div className="active-item-list">
                                            {globalStatus.verification.active_downloads.map(dl => (
                                                <div key={dl.id} className="active-item-mini">
                                                    <span className="mini-filename">{dl.name}</span>
                                                    <span className="mini-url">{dl.location}</span>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            )}

                            {globalStatus.videos?.is_running && (
                                <div className="monitor-section">
                                    <h4>Video Verification</h4>
                                    <div className="activity-item">
                                        <div className="activity-info">
                                            <span>Progress</span>
                                            <span>{globalStatus.videos.processed} / {globalStatus.videos.total_videos}</span>
                                        </div>
                                        <div className="progress-container">
                                            <div
                                                className="progress-bar"
                                                style={{ width: `${(globalStatus.videos.processed / globalStatus.videos.total_videos) * 100}%` }}
                                            ></div>
                                        </div>
                                        <div className="progress-stats">
                                            <span>Found: {globalStatus.videos.missing_found}</span>
                                            <span>Recovered: {globalStatus.videos.recovered}</span>
                                            <span>Active: {globalStatus.videos.active}/{globalStatus.videos.max_allowed}</span>
                                        </div>
                                    </div>
                                    {globalStatus.videos.active_downloads?.length > 0 && (
                                        <div className="active-item-list">
                                            {globalStatus.videos.active_downloads.map(dl => (
                                                <div key={dl.id} className="active-item-mini">
                                                    <span className="mini-filename">{dl.name}</span>
                                                    <span className="mini-url">{dl.location}</span>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            )}

                            {globalStatus.crawler?.active_sources?.length > 0 && (
                                <div className="monitor-section">
                                    <h4>Active Crawls</h4>
                                    {globalStatus.crawler.active_sources.map(source => (
                                        <div key={source.id} className="activity-item">
                                            <div className="activity-info">
                                                <div className="source-name-url">
                                                    <span className="source-name">{source.name}</span>
                                                    <span className="source-url-small">{source.location}</span>
                                                </div>
                                                <span>{source.downloaded_items} / {source.total_items}</span>
                                            </div>
                                            <div className="progress-container">
                                                <div
                                                    className="progress-bar"
                                                    style={{ width: `${source.download_progress || 0}%` }}
                                                ></div>
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}
                </div>
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
                                    <span className={`priority-badge priority-${source.priority || 0}`}>
                                        P{source.priority || 0}
                                    </span>
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

                                {source.status === 'crawling' && (
                                    <div className="source-progress">
                                        <div className="progress-container">
                                            <div
                                                className="progress-bar"
                                                style={{ width: `${source.download_progress || 0}%` }}
                                            ></div>
                                        </div>
                                        <div className="progress-stats">
                                            <span>Progress: {source.download_progress || 0}%</span>
                                            <span>Downloaded: {source.downloaded_items || 0} / {source.total_items || 0}</span>
                                        </div>
                                    </div>
                                )}

                                <div className="source-priority-control">
                                    <label>Priority:</label>
                                    <select
                                        className="priority-select"
                                        value={source.priority || 0}
                                        onChange={(e) => handlePriorityChange(source.id, e.target.value)}
                                        onClick={(e) => e.stopPropagation()}
                                    >
                                        <option value="0">0 (Default)</option>
                                        <option value="1">1 (Low)</option>
                                        <option value="2">2 (Medium)</option>
                                        <option value="3">3 (High)</option>
                                    </select>
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
