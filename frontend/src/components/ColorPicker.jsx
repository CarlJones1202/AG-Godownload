import { useState } from 'react'
import './ColorPicker.css'

function ColorPicker({ onColorSelect }) {
    const [selectedColor, setSelectedColor] = useState('#646cff')

    // Predefined color palette for quick selection
    const presetColors = [
        '#FF0000', '#FF4500', '#FF8C00', '#FFD700', '#FFFF00', '#9ACD32',
        '#00FF00', '#00FA9A', '#00CED1', '#1E90FF', '#0000FF', '#8A2BE2',
        '#FF00FF', '#FF1493', '#FF69B4', '#FFC0CB', '#FFFFFF', '#D3D3D3',
        '#808080', '#000000', '#8B4513', '#A0522D', '#D2691E', '#CD853F'
    ]

    const handleColorChange = (color) => {
        setSelectedColor(color)
        if (onColorSelect) {
            onColorSelect(color)
        }
    }

    const handleHexInput = (e) => {
        const value = e.target.value
        if (/^#[0-9A-Fa-f]{0,6}$/.test(value)) {
            setSelectedColor(value)
            if (value.length === 7) {
                handleColorChange(value)
            }
        }
    }

    return (
        <div className="color-picker">
            <div className="color-preview-section">
                <div
                    className="color-preview"
                    style={{ backgroundColor: selectedColor }}
                />
                <div className="color-input-group">
                    <label>Hex Color</label>
                    <input
                        type="text"
                        className="hex-input"
                        value={selectedColor}
                        onChange={handleHexInput}
                        placeholder="#000000"
                        maxLength={7}
                    />
                </div>
            </div>

            <div className="color-input-section">
                <label>Pick a Color</label>
                <input
                    type="color"
                    className="native-color-picker"
                    value={selectedColor}
                    onChange={(e) => handleColorChange(e.target.value)}
                />
            </div>

            <div className="preset-colors-section">
                <label>Quick Colors</label>
                <div className="preset-colors-grid">
                    {presetColors.map((color) => (
                        <button
                            key={color}
                            className={`preset-color ${selectedColor.toUpperCase() === color ? 'selected' : ''}`}
                            style={{ backgroundColor: color }}
                            onClick={() => handleColorChange(color)}
                            title={color}
                        />
                    ))}
                </div>
            </div>

            <div className="color-picker-actions">
                <button
                    className="search-by-color-btn"
                    onClick={() => onColorSelect && onColorSelect(selectedColor)}
                >
                    Search Similar Colors
                </button>
            </div>
        </div>
    )
}

export default ColorPicker
