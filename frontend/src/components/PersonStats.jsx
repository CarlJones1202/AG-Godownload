import { useState, useEffect } from 'react'
import './PersonStats.css'

function PersonStats({ personId }) {
    const [stats, setStats] = useState(null)
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState(null)

    useEffect(() => {
        setLoading(true)
        fetch(`/api/people/${personId}/stats`)
            .then(res => {
                if (!res.ok) throw new Error('Failed to fetch stats')
                return res.json()
            })
            .then(data => {
                setStats(data)
                setLoading(false)
            })
            .catch(err => {
                console.error(err)
                setError(err.message)
                setLoading(false)
            })
    }, [personId])

    if (loading) return <div className="stats-loading">Loading stats...</div>
    if (error) return null // Hide silently on error as it's optional

    // Calculate max count for chart scaling
    const maxTagCount = stats.top_tags && stats.top_tags.length > 0
        ? Math.max(...stats.top_tags.map(t => t.count))
        : 0

    return (
        <div className="person-stats">
            <h3 className="stats-header">Statistics</h3>

            <div className="stats-grid">
                {/* Overview Cards */}
                <div className="stat-card">
                    <div className="stat-icon">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <polygon points="23 7 16 12 23 17 23 7"></polygon>
                            <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
                        </svg>
                    </div>
                    <div className="stat-content">
                        <span className="stat-value">{stats.video_count}</span>
                        <span className="stat-label">Videos</span>
                    </div>
                </div>

                <div className="stat-card">
                    <div className="stat-icon">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
                            <circle cx="8.5" cy="8.5" r="1.5"></circle>
                            <polyline points="21 15 16 10 5 21"></polyline>
                        </svg>
                    </div>
                    <div className="stat-content">
                        <span className="stat-value">{stats.image_count}</span>
                        <span className="stat-label">Photos</span>
                    </div>
                </div>

                <div className="stat-card">
                    <div className="stat-icon">
                        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                            <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path>
                        </svg>
                    </div>
                    <div className="stat-content">
                        <span className="stat-value">{stats.gallery_count}</span>
                        <span className="stat-label">Galleries</span>
                    </div>
                </div>
            </div>

            {/* Top Tags Chart */}
            {stats.top_tags && stats.top_tags.length > 0 && (
                <div className="tags-chart-section">
                    <h4>Top Tags</h4>
                    <div className="tags-chart">
                        {stats.top_tags.map(tag => (
                            <div key={tag.tag_id} className="chart-row">
                                <div className="chart-label">{tag.name}</div>
                                <div className="chart-bar-container">
                                    <div
                                        className="chart-bar"
                                        style={{ width: `${(tag.count / maxTagCount) * 100}%` }}
                                        title={`${tag.count} items`}
                                    >
                                        <span className="bar-value">{tag.count}</span>
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            )}
        </div>
    )
}

export default PersonStats
