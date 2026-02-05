// Admin UI E2E test using Playwright
const { chromium } = require('playwright');

const BASE_URL = 'http://localhost:8080';
const ADMIN_USERNAME = 'admin';
const ADMIN_PASSWORD = 'XXKokg60kd8hLXgq';

(async () => {
  console.log('\n' + '='.repeat(60));
  console.log('Testing Admin UI with Playwright');
  console.log('='.repeat(60) + '\n');

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  // Capture console messages
  page.on('console', msg => console.log(`   [Console ${msg.type()}] ${msg.text()}`));

  try {
    console.log('1. Loading login page...');
    await page.goto(`${BASE_URL}/admin/login`);
    await page.waitForLoadState('networkidle');

    await page.screenshot({ path: '/tmp/admin_login.png', fullPage: true });
    console.log('   ✓ Screenshot: /tmp/admin_login.png');
    console.log(`   Title: ${await page.title()}`);

    // Check form elements
    const usernameVisible = await page.locator('#username').isVisible();
    const passwordVisible = await page.locator('#password').isVisible();
    const submitVisible = await page.locator('button[type="submit"]').isVisible();

    console.log(`   Username field: ${usernameVisible ? '✓' : '✗'}`);
    console.log(`   Password field: ${passwordVisible ? '✓' : '✗'}`);
    console.log(`   Submit button: ${submitVisible ? '✓' : '✗'}`);

    console.log('\n2. Attempting login...');
    await page.fill('#username', ADMIN_USERNAME);
    await page.fill('#password', ADMIN_PASSWORD);
    console.log(`   Filled username: ${ADMIN_USERNAME}`);
    console.log(`   Filled password: ${'*'.repeat(ADMIN_PASSWORD.length)}`);

    await page.click('button[type="submit"]');
    console.log('   Clicked submit button');

    // Wait for navigation
    await page.waitForTimeout(2000);
    await page.waitForLoadState('networkidle');

    console.log(`\n   Current URL: ${page.url()}`);
    await page.screenshot({ path: '/tmp/admin_after_login.png', fullPage: true });
    console.log('   ✓ Screenshot: /tmp/admin_after_login.png');

    if (page.url().includes('dashboard')) {
      console.log('   ✓ Login successful! Redirected to dashboard\n');

      console.log('3. Testing Dashboard page...');
      console.log(`   Title: ${await page.title()}`);

      // Wait for JavaScript to load stats
      await page.waitForTimeout(2000);

      await page.screenshot({ path: '/tmp/admin_dashboard.png', fullPage: true });
      console.log('   ✓ Screenshot: /tmp/admin_dashboard.png');

      // Check for stats elements
      const pendingCountVisible = await page.locator('#pending-count').isVisible();
      const totalEventsVisible = await page.locator('#total-events').isVisible();

      if (pendingCountVisible) {
        const text = (await page.locator('#pending-count').innerText()).trim();
        console.log(`   Pending Reviews: ${text}`);
      } else {
        console.log('   ⚠ Pending count not visible');
      }

      if (totalEventsVisible) {
        const text = (await page.locator('#total-events').innerText()).trim();
        console.log(`   Total Events: ${text}`);
      } else {
        console.log('   ⚠ Total events not visible');
      }

      // Check navigation
      const navLinksCount = await page.locator('nav a, .navbar a').count();
      console.log(`   Navigation links found: ${navLinksCount}`);

      console.log('\n4. Testing Events List page...');
      await page.goto(`${BASE_URL}/admin/events`);
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      await page.screenshot({ path: '/tmp/admin_events.png', fullPage: true });
      console.log('   ✓ Screenshot: /tmp/admin_events.png');
      console.log(`   Title: ${await page.title()}`);
      console.log(`   URL: ${page.url()}`);

      const eventsHeading = await page.locator('h2:has-text("Events")').count();
      if (eventsHeading > 0) {
        console.log('   ✓ Events heading found');
      }

      console.log('\n5. Testing Duplicates page...');
      await page.goto(`${BASE_URL}/admin/duplicates`);
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      await page.screenshot({ path: '/tmp/admin_duplicates.png', fullPage: true });
      console.log('   ✓ Screenshot: /tmp/admin_duplicates.png');
      console.log(`   Title: ${await page.title()}`);

      console.log('\n6. Testing API Keys page...');
      await page.goto(`${BASE_URL}/admin/api-keys`);
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      await page.screenshot({ path: '/tmp/admin_api_keys.png', fullPage: true });
      console.log('   ✓ Screenshot: /tmp/admin_api_keys.png');
      console.log(`   Title: ${await page.title()}`);

      console.log('\n7. Testing Logout functionality...');
      const logoutCount = await page.locator('button:has-text("Logout"), a:has-text("Logout")').count();

      if (logoutCount > 0) {
        console.log(`   Logout button found: ${logoutCount} instances`);
        const logoutVisible = await page.locator('button:has-text("Logout"), a:has-text("Logout")').first().isVisible();
        console.log(`   Logout button visible: ${logoutVisible ? '✓' : '✗'}`);

        await page.locator('button:has-text("Logout"), a:has-text("Logout")').first().click();
        await page.waitForTimeout(1000);
        await page.waitForLoadState('networkidle');

        console.log(`   After logout URL: ${page.url()}`);
        await page.screenshot({ path: '/tmp/admin_after_logout.png', fullPage: true });
        console.log('   ✓ Screenshot: /tmp/admin_after_logout.png');

        if (page.url().includes('login')) {
          console.log('   ✓ Logout successful - redirected to login');
        } else {
          console.log(`   ⚠ Logout may not have worked - still on: ${page.url()}`);
        }
      } else {
        console.log('   ⚠ Logout button not found');
      }

      console.log('\n' + '='.repeat(60));
      console.log('✓ All tests completed successfully!');
      console.log('Screenshots saved to /tmp/admin_*.png');
      console.log('='.repeat(60) + '\n');

      process.exit(0);

    } else {
      console.log('   ✗ Login failed - not redirected to dashboard');

      // Check for error message
      const errorVisible = await page.locator('#error-message').isVisible();
      if (errorVisible) {
        const errorText = await page.locator('#error-message').innerText();
        console.log(`   Error: ${errorText}`);
      }

      process.exit(1);
    }

  } catch (error) {
    console.log(`\n✗ Error during testing: ${error.message}`);
    await page.screenshot({ path: '/tmp/admin_error.png', fullPage: true });
    console.log('   Error screenshot: /tmp/admin_error.png');
    process.exit(1);

  } finally {
    await browser.close();
  }
})();
