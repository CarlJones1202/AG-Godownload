import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import ScanResultCard from './ScanResultCard'
import './AdminMissingGalleries.css'

function AdminMissingGalleries() {
  const [missingGalleries, setMissingGalleries] = useState([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [providerFilter, setProviderFilter] = useState('all')
  const [linking, setLinking] = useState({})
  const [excluding, setExcluding] = useState({})
  const navigate = useNavigate()

  const fetchMissingGalleries = async () => {
    try {
      const res = await fetch('/api/admin/missing-galleries')
      const data = await res.json()
      setMissingGalleries(data || [])
    } catch (err) {
      console.error('Failed to fetch missing galleries:', err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchMissingGalleries()
  }, [])

  const linkGallery = async (gallery) => {
    const key = gallery.gallery_url
    setLinking(prev => ({ ...prev, [key]: true }))

    try {
      const res = await fetch(`/api/people/${gallery.person_id}/link-found-gallery`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: gallery.provider,
          source_url: gallery.gallery_url,
          name: gallery.gallery_name,
          thumbnail_url: gallery.thumbnail
        })
      })
      if (res.ok) {
        setMissingGalleries(prev => prev.filter(g => g.gallery_url !== gallery.gallery_url))
      }
    } catch (err) {
      console.error('Failed to link gallery:', err)
    } finally {
      setLinking(prev => ({ ...prev, [key]: false }))
    }
  }

  const excludeGallery = async (gallery) => {
    const key = gallery.gallery_url
    setExcluding(prev => ({ ...prev, [key]: true }))

    try {
      const res = await fetch(`/api/people/${gallery.person_id}/exclude-scan-result`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: gallery.provider,
          source_id: '',
          source_url: gallery.gallery_url,
          title: gallery.gallery_name,
          reason: 'rejected_by_admin'
        })
      })
      if (res.ok) {
        setMissingGalleries(prev => prev.filter(g => g.gallery_url !== gallery.gallery_url))
      }
    } catch (err) {
      console.error('Failed to exclude gallery:', err)
    } finally {
      setExcluding(prev => ({ ...prev, [key]: false }))
    }
  }

  const goToPerson = (personId) => {
    navigate(`/people/${personId}`)
  }

  const providers = [...new Set(missingGalleries.map(g => g.provider))]

  const filteredGalleries = missingGalleries.filter(g => {
    const matchesSearch = filter === '' || 
      g.person_name.toLowerCase().includes(filter.toLowerCase()) ||
      g.gallery_name.toLowerCase().includes(filter.toLowerCase())
    const matchesProvider = providerFilter === 'all' || g.provider === providerFilter
    return matchesSearch && matchesProvider
  })

  const groupedByPerson = filteredGalleries.reduce((acc, g) => {
    const key = `${g.person_id}-${g.provider}`
    if (!acc[key]) {
      acc[key] = {
        personId: g.person_id,
        personName: g.person_name,
        provider: g.provider,
        alias: g.alias,
        foundCount: g.found_count,
        missingCount: g.missing_count,
        galleries: []
      }
    }
    acc[key].galleries.push(g)
    return acc
  }, {})

  if (loading) return <div className="admin-loading">Loading...</div>

  return (
    <div className="admin-missing-galleries">
      <div className="admin-header">
        <h2>Missing Galleries</h2>
        <div className="admin-filters">
          <input
            type="text"
            placeholder="Search by person or gallery..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="admin-search"
          />
          <select
            value={providerFilter}
            onChange={(e) => setProviderFilter(e.target.value)}
            className="admin-select"
          >
            <option value="all">All Providers</option>
            {providers.map(p => (
              <option key={p} value={p}>{p}</option>
            ))}
          </select>
          <span className="admin-count">
            {filteredGalleries.length} galleries
          </span>
        </div>
      </div>

      {Object.keys(groupedByPerson).length === 0 ? (
        <div className="admin-empty">
          {missingGalleries.length === 0 
            ? 'No missing galleries found. Run scans on person pages to discover missing galleries.'
            : 'No galleries match your filters.'}
        </div>
      ) : (
        <div className="admin-person-list">
          {Object.values(groupedByPerson).map(group => (
            <div key={`${group.personId}-${group.provider}`} className="admin-person-section">
              <div className="admin-person-header">
                <div className="admin-person-info">
                  <button 
                    className="admin-person-link"
                    onClick={() => goToPerson(group.personId)}
                  >
                    {group.personName}
                  </button>
                  <span className="admin-provider-tag">{group.provider}</span>
                  {group.alias && <span className="admin-alias-tag">{group.alias}</span>}
                </div>
                <div className="admin-person-stats">
                  <span className="stat-found">{group.foundCount} found</span>
                  <span className="stat-missing">{group.galleries.length} missing</span>
                </div>
              </div>
              
              <div className="gallery-cards-grid">
                {group.galleries.map((gallery, idx) => (
                  <div key={`${gallery.gallery_url}-${idx}`} className="gallery-card-grid-item">
                    <ScanResultCard
                      gallery={{
                        url: gallery.gallery_url,
                        title: gallery.gallery_name,
                        thumbnail: gallery.thumbnail,
                        provider: gallery.provider
                      }}
                      provider={gallery.provider}
                      onLink={() => linkGallery(gallery)}
                      onReject={() => excludeGallery(gallery)}
                      isLinking={linking[gallery.gallery_url]}
                      isExcluding={excluding[gallery.gallery_url]}
                      titleLines={2}
                    />
                    <button 
                      className="admin-view-person-btn"
                      onClick={() => goToPerson(gallery.person_id)}
                      title="View person page"
                    >
                      →
                    </button>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default AdminMissingGalleries
