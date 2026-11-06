/**
 * 运行时品牌配置。
 *
 * Vite 会把 public/ 原样复制到 dist/，所以部署后直接改 dist/app-config.js
 * 并刷新页面即可生效，无需重新构建。也可以在 public/app-config.js 修改后
 * 重新构建。注意：这是公开配置，不要在这里放密钥。
 *
 * appName: 浏览器标题、侧栏、登录页显示的产品名。
 * appMark: 品牌方块里的字符；留空时取 appName 首字符。
 */
window.__AEUS_CONFIG__ = {
  appName: 'Board',
  appMark: 'B',
}
