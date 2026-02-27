import { useState, useEffect } from 'react'
import './PersonMissingGalleries.css'

function PersonMissingGalleries({ personId, onScanComplete }) {
  const [scans, setScans] = useState([])
  const [loading, setLoading] = useState(true)
  const [scanning, setScanning] = useState(false)
  const [adding, setAdding] = useState({}) // track which galleries are being added

  const fetchScans = async () => {
    try {
      const res = await fetch(`/api/people/${personId}/scans`)
      const data = await res.json()
      setScans(data || [])
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (personId) fetchScans()
  }, [personId])

  const triggerScan = async () => {
    setScanning(true)
    try {
      await fetch(`/api/people/${personId}/scan`, { method: 'POST' })
      // Poll for completion
      const poll = setInterval(async () => {
        await fetchScans()
        const latest = scans[0]
        if (latest && (latest.status === 'completed' || latest.status === 'failed')) {
          clearInterval(poll)
          setScanning(false)
          if (onScanComplete) onScanComplete()
        }
      }, 2000)
    } catch {
      setScanning(false)
    }
  }

  const linkUnsureGallery = async (gallery, provider, scanId) => {
    const key = `${provider}-unsure-${gallery.url}`
    setAdding(prev => ({ ...prev, [key]: true }))
    try {
      const res = await fetch(`/api/people/${personId}/link-unsure-gallery`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          gallery_id: gallery.id,
          provider: provider,
          source_url: gallery.url
        })
      })
      if (res.ok) {
        // Refresh scans to get updated state after linking
        await fetchScans()
        // Notify parent that something changed
        if (onScanComplete) onScanComplete()
      }
    } catch (err) {
      console.error('Failed to link gallery:', err)
    } finally {
      setAdding(prev => ({ ...prev, [key]: false }))
    }
  }

  if (loading) return <div className="provider-aliases-loading">Loading…</div>

  const completedScans = scans.filter(s => s.status === 'completed')

  return (
    <section className="provider-aliases-section" aria-label="Missing galleries">
      <div className="provider-aliases-header">
        <h3>Missing Galleries</h3>
        <button 
          className="action-btn primary" 
          onClick={triggerScan}
          disabled={scanning}
        >
          {scanning ? 'Scanning...' : 'Scan Now'}
        </button>
      </div>

      {completedScans.length === 0 ? (
        <div className="provider-aliases-empty">
          No scans completed yet. Click "Scan Now" to search for missing galleries.
        </div>
      ) : (
        <div className="provider-aliases-list" style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          {completedScans.map(scan => {
            const results = scan.results || {}
            const missing = results.missing_galleries || []
            const unsure = results.unsure_galleries || []

            return (
              <div key={scan.id} className="scan-result">
                <div className="scan-result-header">
                  <span className="provider-badge">{scan.provider}</span>
                  <span className="scan-meta">
                    {results.found_count || 0} found, {missing.length} missing, {unsure.length} unsure
                  </span>
                  {scan.completed_at && (
                    <span className="scan-date">
                      {new Date(scan.completed_at).toLocaleDateString()}
                    </span>
                  )}
                </div>

                {missing.length > 0 && (
                  <div className="missing-galleries">
                    <h4>Missing ({missing.length})</h4>
                    <div className="gallery-cards-grid">
                      {missing.map((g, i) => (
                        <a 
                          key={i}
                          href={g.url} 
                          target="_blank" 
                          rel="noopener noreferrer"
                          className="missing-gallery-card"
                        >
                          <div className="gallery-thumbnail">
                            {g.thumbnail ? (
                              <img src={g.thumbnail} alt={g.title} />
                            ) : (
                              <div className="no-image">No Thumbnail</div>
                            )}
                          </div>
                          <div className="gallery-info">
                            <h3>{g.title}</h3>
                          </div>
                        </a>
                      ))}
                    </div>
                  </div>
                )}

                {unsure.length > 0 && (
                  <div className="unsure-galleries">
                    <h4>Unsure ({unsure.length})</h4>
                    <div className="gallery-cards-grid">
                      {unsure.map((g, i) => {
                        const key = `${scan.provider}-unsure-${g.url}`
                        return (
                          <div 
                            key={i}
                            className="unsure-gallery-card-wrapper"
                          >
                            <a 
                              href={g.url} 
                              target="_blank" 
                              rel="noopener noreferrer"
                              className="unsure-gallery-card"
                            >
                              <div className="gallery-thumbnail">
                                {g.thumbnail ? (
                                  <img src={g.thumbnail} alt={g.title} />
                                ) : (
                                  <div className="no-image">No Thumbnail</div>
                                )}
                              </div>
                              <div className="gallery-info">
                                <h3>{g.title}</h3>
                              </div>
                            </a>
                            <button 
                              className="link-gallery-btn"
                              onClick={() => linkUnsureGallery(g, scan.provider, scan.id)}
                              disabled={adding[key]}
                              title="Link to this person"
                            >
                              {adding[key] ? '...' : '→'}
                            </button>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}

export default PersonMissingGalleries
