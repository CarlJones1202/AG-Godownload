import React, { useRef, useEffect, useState } from 'react'
import { Canvas, useFrame, useThree } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import * as THREE from 'three'

function VRVideoMesh({ videoElement, mode }) {
    const meshRef = useRef()
    const [texture, setTexture] = useState(null)

    useEffect(() => {
        if (videoElement) {
            const videoTexture = new THREE.VideoTexture(videoElement)
            videoTexture.colorSpace = THREE.SRGBColorSpace
            setTexture(videoTexture)
            return () => {
                videoTexture.dispose()
            }
        }
    }, [videoElement])

    if (!texture) return null

    // Geometry based on mode
    // 360: Full sphere
    // 180: Half sphere (often side-by-side, but assumption here is a single 180 stream or "magic window" style for mono 180)

    // Note: For typical 180 VR videos (like from a VR180 camera), they are often stereoscopic (side-by-side or top-bottom).
    // If it's monoscopic 180 (fisheye), we map to a half sphere.
    // If we want to support simple playback without headset, rendering to a half-sphere in front of the camera is good.

    const is180 = mode === '180'
    const phiLength = is180 ? Math.PI : Math.PI * 2
    const thetaLength = is180 ? Math.PI : Math.PI

    return (
        <mesh ref={meshRef} scale={[-1, 1, 1]} rotation={[0, is180 ? -Math.PI / 2 : 0, 0]}>
            <sphereGeometry args={[500, 60, 40, 0, phiLength, 0, thetaLength]} />
            <meshBasicMaterial map={texture} side={THREE.BackSide} />
        </mesh>
    )
}

function VRPlayer({ videoElement, mode }) {
    if (!videoElement) return null

    return (
        <div style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', zIndex: 10 }}>
            <Canvas camera={{ position: [0, 0, 0.1], fov: 75 }}>
                <VRVideoMesh videoElement={videoElement} mode={mode} />
                <OrbitControls
                    enableZoom={true}
                    enablePan={false}
                    rotateSpeed={-0.5} // Invert rotation for "looking around" feel
                    reverseOrbit={true}
                />
            </Canvas>
        </div>
    )
}

export default VRPlayer
