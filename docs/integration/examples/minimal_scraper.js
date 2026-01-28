// Minimal Event Scraper Example
// Submit events to SEL with automatic duplicate detection

const API_KEY = process.env.SEL_API_KEY;
const BASE_URL = process.env.SEL_BASE_URL || 'https://sel.togather.events';

async function submitEvent(event) {
  const response = await fetch(`${BASE_URL}/api/v1/events`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`
    },
    body: JSON.stringify(event)
  });
  
  const data = await response.json();
  
  if (response.status === 201) {
    console.log('✓ Event created:', data['@id']);
    return data;
  } else if (response.status === 409) {
    console.log('ℹ Event already exists:', data['@id']);
    return data;
  } else {
    console.error('✗ Error:', response.status, data.detail);
    throw new Error(`Failed to submit event: ${data.detail}`);
  }
}

// Example: Submit a minimal event
const minimalEvent = {
  name: 'Comedy Night at The Tranzac',
  startDate: '2026-02-15T20:00:00-05:00',
  location: {
    name: 'The Tranzac',
    addressLocality: 'Toronto',
    addressRegion: 'ON'
  },
  source: {
    url: 'https://thetranzac.com/events/comedy-night'
  }
};

submitEvent(minimalEvent)
  .then(event => console.log('Success!', event['@id']))
  .catch(err => console.error('Failed:', err.message));

// Example: Submit with more details
const detailedEvent = {
  name: 'Jazz Night at The Rex',
  description: 'Live jazz performance featuring local artists',
  startDate: '2026-02-20T20:00:00-05:00',
  endDate: '2026-02-20T23:00:00-05:00',
  location: {
    name: 'The Rex Hotel Jazz & Blues Bar',
    streetAddress: '194 Queen St W',
    addressLocality: 'Toronto',
    addressRegion: 'ON',
    postalCode: 'M5V 1Z1',
    addressCountry: 'CA'
  },
  organizer: {
    name: 'The Rex Hotel'
  },
  offers: {
    price: '15.00',
    priceCurrency: 'CAD',
    url: 'https://therex.ca/tickets'
  },
  url: 'https://therex.ca/events/jazz-night',
  image: 'https://therex.ca/images/jazz-night.jpg',
  source: {
    url: 'https://therex.ca/events/jazz-night',
    eventId: 'jazz-2026-02-20',
    name: 'Rex Hotel Scraper'
  }
};

submitEvent(detailedEvent)
  .then(event => console.log('Success!', event['@id']))
  .catch(err => console.error('Failed:', err.message));
