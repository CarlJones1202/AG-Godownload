import React from 'react';
import './SortControls.css';

function SortControls({ sort, setSort, seed, setSeed, onRandomize }) {
    const handleRandomize = () => {
        const newSeed = Math.floor(Math.random() * 1000000);
        onRandomize(newSeed);
    };

    return (
        <div className="sort-controls">
            <div className="control-group">
                <label>Sort:</label>
                <select value={sort} onChange={(e) => setSort(e.target.value)}>
                    <option value="newest">Newest</option>
                    <option value="oldest">Oldest</option>
                    <option value="shuffle">Shuffle</option>
                    {/* Add more as needed by specific lists */}
                </select>
            </div>

            {sort === 'shuffle' && (
                <div className="control-group animate-in">
                    <label>Seed:</label>
                    <div className="seed-input-wrapper">
                        <input
                            type="number"
                            value={seed}
                            onChange={(e) => setSeed(parseInt(e.target.value) || 0)}
                            className="seed-input"
                        />
                        <button
                            className="randomize-btn"
                            onClick={handleRandomize}
                            title="Generate random seed"
                        >
                            🎲
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}

export default SortControls;
