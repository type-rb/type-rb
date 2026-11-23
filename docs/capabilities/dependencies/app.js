const rows = [...document.querySelectorAll('tbody tr[data-status]')];
const status = document.querySelector('#status');
const search = document.querySelector('#search');
function filter() {
  const query = search.value.trim().toLowerCase();
  let count = 0;
  for (const row of rows) {
    row.hidden = (status.value !== 'all' && row.dataset.status !== status.value)
      || !row.textContent.toLowerCase().includes(query);
    if (!row.hidden) count += 1;
  }
  document.querySelector('#visible').textContent = `${count} of ${rows.length} entries`;
}
status.addEventListener('change', filter);
search.addEventListener('input', filter);
filter();
