const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch({ 
        headless: false,
        executablePath: 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
    });
    const context = await browser.newContext();
    const page = await context.newPage();
    
    await page.goto('http://127.0.0.1:5000/terminal');
    await page.waitForTimeout(3000);
    
    // 检查串口下拉框的实际值
    const portSelect = await page.$('#port-select');
    const selectedValue = await portSelect.evaluate(el => el.value);
    const options = await page.$$eval('#port-select option', opts => 
        opts.map(o => ({ value: o.value, text: o.text, selected: o.selected }))
    );
    
    console.log('串口下拉框当前值:', selectedValue);
    console.log('所有选项:', JSON.stringify(options, null, 2));
    
    // 检查下拉框是否可见
    const isVisible = await portSelect.isVisible();
    console.log('下拉框是否可见:', isVisible);
    
    // 截图
    await page.screenshot({ path: 'port-list-check.png', fullPage: true });
    console.log('截图已保存: port-list-check.png');
    
    await browser.close();
})();