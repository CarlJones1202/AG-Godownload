import { useState, useEffect } from 'react'
import './PersonMissingGalleries.css'

function PersonMissingGalleries({ personId, onScanComplete }) {
  const [scans, setScans] = useState([])
  const [loading, setLoading] = useState(true)
  const [scanning, setScanning] = useState(false)
  const [adding, setAdding] = useState({}) // track which galleries are being added
  const [excluding, setExcluding] = useState({}) // track which galleries are being excluded

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
    
    // Optimistically remove the gallery from the unsure list
    setScans(prevScans =>
      prevScans.map(scan => {
        if (scan.id === scanId) {
          const results = scan.results || {}
          const unsureGalleries = results.unsure_galleries || []
          return {
            ...scan,
            results: {
              ...results,
              unsure_count: unsureGalleries.length - 1,
              unsure_galleries: unsureGalleries.filter(
                g => g.url !== gallery.url
              )
            }
          }
        }
        return scan
      })
    )

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
      if (!res.ok) {
        console.error('Failed to link gallery')
      }
    } catch (err) {
      console.error('Failed to link gallery:', err)
    } finally {
      setAdding(prev => ({ ...prev, [key]: false }))
    }
  }

  const excludeScanResult = async (gallery, provider, scanId, isUnsure) => {
    const key = `${provider}-${isUnsure ? 'unsure' : 'missing'}-${gallery.url}`
    setExcluding(prev => ({ ...prev, [key]: true }))
    
    // Optimistically remove the gallery from the list
    setScans(prevScans =>
      prevScans.map(scan => {
        if (scan.id === scanId) {
          const results = scan.results || {}
          if (isUnsure) {
            const unsureGalleries = results.unsure_galleries || []
            return {
              ...scan,
              results: {
                ...results,
                unsure_count: unsureGalleries.length - 1,
                unsure_galleries: unsureGalleries.filter(
                  g => g.url !== gallery.url
                )
              }
            }
          } else {
            const missingGalleries = results.missing_galleries || []
            return {
              ...scan,
              results: {
                ...results,
                missing_count: missingGalleries.length - 1,
                missing_galleries: missingGalleries.filter(
                  g => g.url !== gallery.url
                )
              }
            }
          }
        }
        return scan
      })
    )

    try {
      const res = await fetch(`/api/people/${personId}/exclude-scan-result`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: provider,
          source_id: gallery.source_id,
          source_url: gallery.url,
          title: gallery.title,
          reason: 'rejected_by_user'
        })
      })
      if (!res.ok) {
        console.error('Failed to exclude gallery')
      }
    } catch (err) {
      console.error('Failed to exclude gallery:', err)
    } finally {
      setExcluding(prev => ({ ...prev, [key]: false }))
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
                      {missing.map((g, i) => {
                        const key = `${scan.provider}-missing-${g.url}`
                        return (
                          <div
                            key={i}
                            className="missing-gallery-card-wrapper"
                          >
                            <a 
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
                            <button 
                              className="reject-gallery-btn"
                              onClick={() => excludeScanResult(g, scan.provider, scan.id, false)}
                              disabled={excluding[key]}
                              title="Reject this result"
                            >
                              {excluding[key] ? '...' : '✕'}
                            </button>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}

                {unsure.length > 0 && (
                  <div className="unsure-galleries">
                    <h4>Unsure ({unsure.length})</h4>
                    <div className="gallery-cards-grid">
                      {unsure.map((g, i) => {
                        const linkKey = `${scan.provider}-unsure-${g.url}`
                        const rejectKey = `${scan.provider}-unsure-reject-${g.url}`
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
                              disabled={adding[linkKey]}
                              title="Link to this person"
                            >
                              {adding[linkKey] ? '...' : '→'}
                            </button>
                            <button 
                              className="reject-gallery-btn"
                              onClick={() => excludeScanResult(g, scan.provider, scan.id, true)}
                              disabled={excluding[rejectKey]}
                              title="Reject this result"
                            >
                              {excluding[rejectKey] ? '...' : '✕'}
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
