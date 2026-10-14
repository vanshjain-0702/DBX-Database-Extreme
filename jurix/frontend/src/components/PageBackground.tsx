import { useEffect, useState } from 'react';

export default function PageBackground({ imageUrl }: { imageUrl: string }) {
    const [images, setImages] = useState<string[]>([]);
    
    useEffect(() => {
        setImages(prev => {
            if (prev[prev.length - 1] === imageUrl) return prev;
            return [...prev.slice(-3), imageUrl];
        });
    }, [imageUrl]);

    return (
        <div className="workspace-background" aria-hidden="true">
            {images.map((img, i) => (
                <div 
                    key={img + i} 
                    className={img === imageUrl ? 'visible' : ''}
                    style={{ backgroundImage: `url(${img})` }} 
                />
            ))}
        </div>
    );
}
