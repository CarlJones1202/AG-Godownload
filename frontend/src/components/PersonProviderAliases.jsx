// Lightweight, modern UI for alias-provider mappings
import { useState, useEffect } from 'react'
import './PersonDetail_provider_aliases.css'

const KNOWN_PROVIDERS = ['metart', 'metartx', 'playboy', 'playboyplus', 'vixen', 'sexart', 'lifeerotic']

function PersonProviderAliases({ personId, aliases: personAliases, personName, onUpdate }) {
  const [providerAliases, setProviderAliases] = useState([])
  const [loading, setLoading] = useState(true)
  const [provider, setProvider] = useState('')
  const [alias, setAlias] = useState('')
  const [custom, setCustom] = useState('')
  const [showAdd, setShowAdd] = useState(false)

  useEffect(() => {
    if (personId) fetchAliases()
  }, [personId])

  const fetchAliases = async () => {
    try {
      const res = await fetch(`/api/people/${personId}/provider-aliases`)
      const data = await res.json()
      setProviderAliases(data.data || [])
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }

  const parseAliases = (a) => {
    try { return JSON.parse(a || '[]') } catch { return [] }
  }
  const allNames = [personName, ...parseAliases(personAliases)].filter(Boolean)

  // Get list of providers already used by this person
  const usedProviders = new Set(providerAliases.map(a => a.provider.toLowerCase()))
  
  // Filter out providers that already have aliases
  const availableProviders = KNOWN_PROVIDERS.filter(p => !usedProviders.has(p))

  const addAlias = async () => {
    const toAlias = (custom || alias).trim()
    if (!provider || !toAlias) return
    // Preserve case for custom, use original for selected
    const finalAlias = custom ? custom.trim() : allNames.find(n => n.toLowerCase() === alias.toLowerCase())
    try {
      const res = await fetch(`/api/people/${personId}/provider-aliases`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider: provider.toLowerCase(), alias: finalAlias })
      })
      if (res.ok) {
        setProvider('')
        setAlias('')
        setCustom('')
        setShowAdd(false)
        fetchAliases()
        if (onUpdate) onUpdate()
      }
    } catch {
      // ignore
    }
  }

  const removeAlias = async (id) => {
    try {
      const res = await fetch(`/api/people/${personId}/provider-aliases/${id}`, { method: 'DELETE' })
      if (res.ok) {
        fetchAliases()
        if (onUpdate) onUpdate()
      }
    } catch {
      // ignore
    }
  }

  if (loading) return <div className="provider-aliases-loading">Loading…</div>

  return (
    <section className="provider-aliases-section" aria-label="Provider aliases">
      <div className="provider-aliases-header">
        <h3>Provider Aliases</h3>
        <button className="action-btn primary" onClick={() => setShowAdd(v => !v)}>
          {showAdd ? 'Close' : '+ Add'}
        </button>
      </div>

      {showAdd && (
        <div className="provider-alias-add-form" aria-label="Add provider alias">
           <select value={provider} onChange={e => setProvider(e.target.value)}>
             <option value="">Provider</option>
             {availableProviders.map(p => (
               <option key={p} value={p}>{p[0].toUpperCase() + p.slice(1)}</option>
             ))}
           </select>
          <select value={alias} onChange={e => setAlias(e.target.value)}>
            <option value="">Name</option>
            {allNames.map(n => (
              <option key={n} value={n.toLowerCase()}>{n}</option>
            ))}
          </select>
          <input placeholder="Custom name" value={custom} onChange={e => setCustom(e.target.value)} />
          <button className="action-btn primary" onClick={addAlias}>Add</button>
        </div>
      )}

      {providerAliases.length === 0 ? (
        <div className="provider-aliases-empty">No provider aliases mapped yet.</div>
      ) : (
        <div className="provider-aliases-list" style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem' }}>
          {providerAliases.map(a => (
            <span key={a.id} className="provider-alias-chip">
              <span className="provider-badge">{a.provider}</span>
              <span className="alias-text">{a.alias}</span>
              <button className="remove-alias" onClick={() => removeAlias(a.id)} aria-label="Remove">×</button>
            </span>
          ))}
        </div>
      )}
    </section>
  )
}

export default PersonProviderAliases
