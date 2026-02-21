import { chromium } from 'playwright';

const browser = await chromium.launch();
const page = await browser.newPage();

// Load a nearby page with known coordinates (University of MN area)
await page.goto('http://localhost:9990/nearby?lat=44.973109&lon=-93.243701&view=routes');
await page.waitForSelector('[data-testid="route-row"]', { timeout: 10000 });

// 1. Check "Change location" is on same line as heading
const header = await page.$('.nearby-header');
const h2 = await page.$('.nearby-header h2');
const changeLink = await page.$('.change-location-link');

if (header && h2 && changeLink) {
  const headerBox = await header.boundingBox();
  const h2Box = await h2.boundingBox();
  const linkBox = await changeLink.boundingBox();

  console.log('Header:', JSON.stringify(headerBox));
  console.log('H2:', JSON.stringify(h2Box));
  console.log('Change location:', JSON.stringify(linkBox));

  // They should be on the same line (similar Y position)
  const sameLine = Math.abs(h2Box.y - linkBox.y) < 20;
  console.log('Same line:', sameLine ? 'YES' : 'NO');

  // Link should be to the right
  const linkIsRight = linkBox.x > h2Box.x + h2Box.width;
  console.log('Link is right of heading:', linkIsRight ? 'YES' : 'NO');
} else {
  console.log('MISSING: header=' + !!header + ' h2=' + !!h2 + ' link=' + !!changeLink);
}

// 2. Check direction toggle works
const rows = await page.$$('[data-testid="route-row"]');
console.log('\nTotal route rows:', rows.length);

let toggleFound = false;
for (const row of rows) {
  const group = await row.$('.direction-group');
  if (!group) continue;

  toggleFound = true;
  const primary = await group.$('.direction-primary');
  const alt = await group.$('.direction-alt');

  const primaryHidden = await primary.getAttribute('hidden');
  const altHidden = await alt.getAttribute('hidden');
  console.log('Before click: primary hidden=' + primaryHidden + ', alt hidden=' + altHidden);

  // Click the row
  await row.click();
  await page.waitForTimeout(100);

  const primaryHidden2 = await primary.getAttribute('hidden');
  const altHidden2 = await alt.getAttribute('hidden');
  console.log('After click:  primary hidden=' + primaryHidden2 + ', alt hidden=' + altHidden2);

  // Toggle should have swapped
  const toggled = primaryHidden2 !== null && altHidden2 === null;
  console.log('Toggle worked:', toggled ? 'YES' : 'NO');

  // Click again to toggle back
  await row.click();
  await page.waitForTimeout(100);

  const primaryHidden3 = await primary.getAttribute('hidden');
  const altHidden3 = await alt.getAttribute('hidden');
  console.log('After 2nd click: primary hidden=' + primaryHidden3 + ', alt hidden=' + altHidden3);
  const toggledBack = primaryHidden3 === null && altHidden3 !== null;
  console.log('Toggle back worked:', toggledBack ? 'YES' : 'NO');

  break;
}

if (!toggleFound) {
  console.log('No rows with direction-group found');
}

// 3. Check badge visibility - get computed styles
const badges = await page.$$('.route-badge-sm');
console.log('\nBadges found:', badges.length);
for (let i = 0; i < Math.min(3, badges.length); i++) {
  const bg = await badges[i].evaluate(el => getComputedStyle(el).backgroundColor);
  const border = await badges[i].evaluate(el => getComputedStyle(el).border);
  const text = await badges[i].textContent();
  console.log(`Badge "${text.trim()}": bg=${bg}, border=${border}`);
}

// 4. Check row separation - get row positions
console.log('\nRow positions:');
for (let i = 0; i < Math.min(3, rows.length); i++) {
  const box = await rows[i].boundingBox();
  const borderBottom = await rows[i].evaluate(el => getComputedStyle(el).borderBottom);
  console.log(`Row ${i}: y=${box.y.toFixed(0)} h=${box.height.toFixed(0)} border-bottom=${borderBottom}`);
}

// 5. Screenshot
await page.screenshot({ path: '/tmp/nearby-test.png', fullPage: true });
console.log('\nScreenshot saved to /tmp/nearby-test.png');

await browser.close();
