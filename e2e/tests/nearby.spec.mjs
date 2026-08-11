import { test, expect } from '@playwright/test';

// Fixture geometry (see serve-fixture.mjs): stops A and B sit within ~50 m of
// this query point; route 10 runs both directions, route 21 one.
const NEARBY = '/nearby?lat=44.9778&lon=-93.2650';

test.describe('nearby routes view', () => {
  test('shows nearby route departures from the fixture schedule', async ({ page }) => {
    await page.goto(NEARBY + '&view=routes');
    const rows = page.getByTestId('route-row');
    await expect(rows.first()).toBeVisible();
    await expect(page.locator('.route-badge-sm').filter({ hasText: '10' }).first()).toBeVisible();
    await expect(page.locator('.route-badge-sm').filter({ hasText: '21' }).first()).toBeVisible();
  });

  test('direction toggle swaps between paired directions', async ({ page }) => {
    await page.goto(NEARBY + '&view=routes');
    const group = page.locator('.direction-group').first();
    await expect(group).toBeVisible();

    const primary = group.locator('.direction-primary');
    const alt = group.locator('.direction-alt');
    await expect(primary).toBeVisible();
    await expect(alt).toBeHidden();

    await group.locator('.direction-toggle').first().click();
    await expect(primary).toBeHidden();
    await expect(alt).toBeVisible();

    await alt.locator('.direction-toggle').first().click();
    await expect(primary).toBeVisible();
    await expect(alt).toBeHidden();
  });
});

test.describe('nearby stops view', () => {
  test('lists the fixture stops with distances', async ({ page }) => {
    await page.goto(NEARBY + '&view=stops');
    const cards = page.getByTestId('stop-card');
    await expect(cards.first()).toBeVisible();
    await expect(page.getByText('Test St & 1st Ave').first()).toBeVisible();
    await expect(page.getByTestId('stop-distance').first()).toBeVisible();
  });

  test('unit toggle switches between metric and imperial', async ({ page }) => {
    await page.goto(NEARBY + '&view=stops');
    const dist = page.getByTestId('stop-distance').first();
    await expect(dist).toContainText(/m |km /);

    await page.locator('#unit-toggle').click();
    await expect(dist).toContainText(/ft|mi/);

    // Preference persists server-side across reloads.
    await page.reload();
    await expect(page.getByTestId('stop-distance').first()).toContainText(/ft|mi/);
    await page.locator('#unit-toggle').click();
    await expect(page.getByTestId('stop-distance').first()).toContainText(/m |km /);
  });
});

test.describe('stop detail', () => {
  test('shows the stop name and scheduled departures', async ({ page }) => {
    await page.goto('/stops/A');
    await expect(page.getByTestId('stop-name')).toContainText('Test St & 1st Ave');
    await expect(page.locator('.route-badge-sm, .route-badge').first()).toBeVisible();
  });

  test('save stop persists to the local DB and survives reload', async ({ page }) => {
    await page.goto('/stops/A');
    const btn = page.locator('#save-stop-btn');
    await expect(btn).toBeVisible();

    page.on('dialog', (dialog) => dialog.accept('Home'));
    if ((await btn.textContent()) !== 'Saved') {
      await btn.click();
    }
    await expect(btn).toHaveText('Saved');

    await page.reload();
    await expect(page.locator('#save-stop-btn')).toHaveText('Saved');

    // Clean up: unsave so the test is re-runnable against the same DB.
    await page.locator('#save-stop-btn').click();
    await expect(page.locator('#save-stop-btn')).toHaveText('Save stop');
  });
});

test.describe('PWA (browser mode)', () => {
  test('serves the manifest and service worker outside the native shell', async ({ page, request }) => {
    const manifest = await request.get('/manifest.json');
    expect(manifest.ok()).toBeTruthy();
    const sw = await request.get('/sw.js');
    expect(sw.ok()).toBeTruthy();

    // Browser pages carry no data-native marker and do link the manifest.
    await page.goto(NEARBY + '&view=routes');
    await expect(page.locator('html')).not.toHaveAttribute('data-native', '1');
    await expect(page.locator('link[rel="manifest"]')).toHaveCount(1);
  });
});
