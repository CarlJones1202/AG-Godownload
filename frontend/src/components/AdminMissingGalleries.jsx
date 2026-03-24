import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import ScanResultCard from './ScanResultCard'
import './AdminMissingGalleries.css'

function ContextMenu({ x, y, onClose, children }) {
  const menuRef = useRef(null)

  useEffect(() => {
    const handleClickOutside = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        onClose()
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [onClose])

  return (
    <div className="context-menu" ref={menuRef} style={{ left: x, top: y }}>
      {children}
    </div>
  )
}

function AdminMissingGalleries() {
  const [missingGalleries, setMissingGalleries] = useState([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const [providerFilter, setProviderFilter] = useState('all')
  const [sortBy, setSortBy] = useState('name')
  const [selectedPerson, setSelectedPerson] = useState('all')
  const [linking, setLinking] = useState({})
  const [excluding, setExcluding] = useState({})
  const [contextMenu, setContextMenu] = useState(null)
  const navigate = useNavigate()

  const fetchMissingGalleries = async (sort) => {
    try {
      const res = await fetch(`/api/admin/missing-galleries?sort=${sort}`)
      const data = await res.json()
      setMissingGalleries(data || [])
    } catch (err) {
      console.error('Failed to fetch missing galleries:', err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    setLoading(true)
    fetchMissingGalleries(sortBy)
  }, [sortBy])

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

  const searchOnViperGirls = (gallery) => {
    const personName = encodeURIComponent(`"${gallery.person_name}"`)
    const albumName = encodeURIComponent(`"${gallery.gallery_name}"`)
    const url = `https://vipergirls.to/search.php?do=process&searchthreadid=&s=&securitytoken=guest&searchfromtype=vBForum%3APost&do=process&contenttypeid=1&query=${personName}+${albumName}&titleonly=0&searchuser=&starteronly=0&tag=&forumchoice%5B%5D=235&childforums=1&replyless=0&replylimit=&searchdate=0&beforeafter=after&sortby=dateline&order=descending&showposts=0&dosearch=Search+Now`
    window.open(url, '_blank')
  }

  const handleContextMenu = (e, gallery) => {
    e.preventDefault()
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      gallery
    })
  }

  const goToPerson = (personId) => {
    navigate(`/people/${personId}`)
  }

  const providers = [...new Set(missingGalleries.map(g => g.provider))]

  const uniquePersons = []
  const personMap = new Map()
  missingGalleries.forEach(g => {
    if (!personMap.has(g.person_id)) {
      personMap.set(g.person_id, {
        personId: g.person_id,
        personName: g.person_name
      })
    }
  })
  personMap.forEach(p => uniquePersons.push(p))
  uniquePersons.sort((a, b) => a.personName.localeCompare(b.personName))

  const filteredGalleries = missingGalleries.filter(g => {
    const matchesSearch = filter === '' || 
      g.person_name.toLowerCase().includes(filter.toLowerCase()) ||
      g.gallery_name.toLowerCase().includes(filter.toLowerCase())
    const matchesProvider = providerFilter === 'all' || g.provider === providerFilter
    const matchesPerson = selectedPerson === 'all' || g.person_id === selectedPerson
    return matchesSearch && matchesProvider && matchesPerson
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
        <div className="person-tabs">
          <button
            className={`person-tab ${selectedPerson === 'all' ? 'active' : ''}`}
            onClick={() => setSelectedPerson('all')}
          >
            All <span className="tab-count">{missingGalleries.length}</span>
          </button>
          {uniquePersons.map(p => {
            const count = missingGalleries.filter(g => g.person_id === p.personId).length
            return (
              <button
                key={p.personId}
                className={`person-tab ${selectedPerson === p.personId ? 'active' : ''}`}
                onClick={() => setSelectedPerson(p.personId)}
              >
                {p.personName} <span className="tab-count">{count}</span>
              </button>
            )
          })}
        </div>
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
          <select
            value={sortBy}
            onChange={(e) => setSortBy(e.target.value)}
            className="admin-select"
          >
            <option value="name">Sort by Name</option>
            <option value="date">Sort by Most Recent</option>
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
                  <div 
                    key={`${gallery.gallery_url}-${idx}`} 
                    className="gallery-card-grid-item"
                    onContextMenu={(e) => handleContextMenu(e, gallery)}
                  >
                    <ScanResultCard
                      gallery={{
                        url: gallery.gallery_url,
                        title: gallery.gallery_name,
                        thumbnail: gallery.thumbnail,
                        provider: gallery.provider,
                        release_date: gallery.release_date
                      }}
                      provider={gallery.provider}
                      titleLines={2}
                    />
                    <button 
                      className="context-menu-trigger"
                      onClick={(e) => {
                        e.stopPropagation()
                        const rect = e.currentTarget.getBoundingClientRect()
                        setContextMenu({
                          x: rect.left,
                          y: rect.bottom + 4,
                          gallery
                        })
                      }}
                      title="More options"
                    >
                      ⋮
                    </button>
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

      {contextMenu && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
        >
          <button
            className="context-menu-item"
            onClick={() => {
              searchOnViperGirls(contextMenu.gallery)
              setContextMenu(null)
            }}
          >
            🔍 Search on ViperGirls
          </button>
          <button
            className="context-menu-item"
            onClick={() => {
              linkGallery(contextMenu.gallery)
              setContextMenu(null)
            }}
            disabled={linking[contextMenu.gallery.gallery_url]}
          >
            {linking[contextMenu.gallery.gallery_url] ? '...' : '→'} Link to Person
          </button>
          <button
            className="context-menu-item danger"
            onClick={() => {
              excludeGallery(contextMenu.gallery)
              setContextMenu(null)
            }}
            disabled={excluding[contextMenu.gallery.gallery_url]}
          >
            {excluding[contextMenu.gallery.gallery_url] ? '...' : '✕'} Reject
          </button>
        </ContextMenu>
      )}
    </div>
  )
}

export default AdminMissingGalleries
