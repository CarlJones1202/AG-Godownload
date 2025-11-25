import { useState } from 'react'
import './ImageGrid.css'
import Lightbox from './Lightbox'

function ImageGrid({ gallery }) {
    const [selectedImage, setSelectedImage] = useState(null)
    const [selectedIndex, setSelectedIndex] = useState(0)

    const handleImageClick = (image, index) => {
        setSelectedImage(image)
        setSelectedIndex(index)
    }

    const handleClose = () => {
        setSelectedImage(null)
    }

    const handleNext = () => {
        const nextIndex = (selectedIndex + 1) % gallery.images.length
        setSelectedIndex(nextIndex)
        setSelectedImage(gallery.images[nextIndex])
    }

    const handlePrev = () => {
        const prevIndex = (selectedIndex - 1 + gallery.images.length) % gallery.images.length
        setSelectedIndex(prevIndex)
        setSelectedImage(gallery.images[prevIndex])
    }

    if (!gallery.images || gallery.images.length === 0) {
        return <div className="empty-state">No images in this gallery yet.</div>
    }

    return (
        <>
            <div className="image-grid">
                {gallery.images.map((image, index) => (
                    <div
                        key={image.id}
                        className="image-card"
                        onClick={() => handleImageClick(image, index)}
                    >
                        <img
                            src={`/api/thumbnails/${image.filename}`}
                            alt={`Image ${image.id}`}
                            loading="lazy"
                        />
                    </div>
                ))}
            </div>

            {selectedImage && (
                <Lightbox
                    image={selectedImage}
                    onClose={handleClose}
                    onNext={handleNext}
                    onPrev={handlePrev}
                    currentIndex={selectedIndex + 1}
                    totalImages={gallery.images.length}
                />
            )}
        </>
    )
}

export default ImageGrid
