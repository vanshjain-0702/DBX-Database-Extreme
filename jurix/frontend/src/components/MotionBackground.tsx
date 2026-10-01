import { useState, useEffect } from 'react';

const IMAGES = [
  '/img/justice.jpg',
  '/img/chambers.jpg',
  '/img/corridor.jpg',
  '/img/briefs.jpg',
  '/img/desk.jpg',
  '/img/lamp.jpg',
  '/img/parchment.jpg',
  '/img/sealpress.jpg',
];

export default function MotionBackground() {
  const [currentIndex, setCurrentIndex] = useState(0);

  useEffect(() => {
    // Change image every 8 seconds
    const interval = setInterval(() => {
      setCurrentIndex((prev) => (prev + 1) % IMAGES.length);
    }, 8000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="absolute inset-0 z-0 overflow-hidden bg-[#0a0806]">
      {IMAGES.map((img, index) => (
        <div
          key={img}
          className={`absolute inset-0 bg-cover bg-center transition-opacity duration-[3000ms] ease-in-out ${
            index === currentIndex ? 'opacity-40 animate-ken-burns' : 'opacity-0'
          }`}
          style={{ 
            backgroundImage: `url("${img}")`,
            // Fallback for animation if tailwind config isn't updated
            animation: index === currentIndex ? 'kenBurns 15s ease-out forwards' : 'none'
          }}
        />
      ))}
      <style>{`
        @keyframes kenBurns {
          0% {
            transform: scale(1);
          }
          100% {
            transform: scale(1.15);
          }
        }
      `}</style>
      <div className="absolute inset-0 bg-gradient-to-t from-[#0a0806] via-[#0a0806]/80 to-transparent mix-blend-multiply pointer-events-none" />
    </div>
  );
}
