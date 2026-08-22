export const fetchWithAuth = async (url: string, options: RequestInit = {}) => {
  const token = localStorage.getItem('dbx_token');

  const headers = new Headers(options.headers || {});
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  
  const res = await fetch(url, { ...options, headers });
  if (res.status === 401) {
    localStorage.removeItem('dbx_token');
    window.location.href = '/login';
  }
  return res;
};
